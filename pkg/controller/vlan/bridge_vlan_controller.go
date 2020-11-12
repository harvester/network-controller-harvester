package vlan

import (
	"context"
	"fmt"

	harvesterv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	harvcontroller "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	cniv1 "github.com/rancher/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"

	"github.com/rancher/harvester-network-controller/pkg/bridge"
	"github.com/rancher/harvester-network-controller/pkg/util"
)

type BridgeVLANController struct {
	settingClient harvcontroller.SettingClient
	settingsCache harvcontroller.SettingCache
	nadCache      cniv1.NetworkAttachmentDefinitionCache
	bridge        *bridge.Bridge
}

const (
	NetworkSettingName       = "network-setting"
	BridgeVlanControllerName = "bridge-vlan-controller"
	BridgeName               = "harvester-br0"
	BridgeCNIName            = "bridge"
)

func Register(ctx context.Context, setting harvcontroller.SettingController, nad cniv1.NetworkAttachmentDefinitionController) error {
	controller := &BridgeVLANController{
		settingClient: setting,
		settingsCache: setting.Cache(),
		nadCache:      nad.Cache(),
		bridge:        bridge.NewBridge(BridgeName),
	}

	if err := initNetworkSettings(controller.settingClient); err != nil {
		return err
	}

	setting.OnChange(ctx, BridgeVlanControllerName, controller.OnChange)
	setting.OnRemove(ctx, BridgeVlanControllerName, controller.OnRemove)

	return nil
}

func (c *BridgeVLANController) OnChange(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	if setting.Value == "" || key != NetworkSettingName {
		return setting, nil
	}

	klog.Infof("harvester network setting configured to: %s", setting.Value)
	networkSetting, err := DecodeNetworkSettings(setting.Value)
	if err != nil {
		return nil, fmt.Errorf("decode failed, error: %w, value: %s", err, setting.Value)
	}

	if networkSetting.NIC == "" && networkSetting.ConfiguredNIC == "" {
		klog.Errorf("skip network config, both NIC and configured NIC are empty")
		return nil, nil
	}

	settingCopy := setting.DeepCopy()
	if err := c.configBridgeNetwork(networkSetting); err != nil {
		harvesterv1.SettingConfigured.False(settingCopy)
		harvesterv1.SettingConfigured.Reason(settingCopy, fmt.Sprintf("failed to config bridge vlan, error:%s", err.Error()))
		if _, err := c.settingClient.Update(settingCopy); err != nil {
			return nil, err
		}
		return nil, err
	}

	networkSetting.ConfiguredNIC = networkSetting.NIC
	settingCopy.Value, err = EncodeNetworkSettings(networkSetting)
	if err != nil {
		return nil, fmt.Errorf("encode network settings failed, error: %w, networksettings: %+v", err, networkSetting)
	}

	harvesterv1.SettingConfigured.True(settingCopy)
	harvesterv1.SettingConfigured.Reason(settingCopy, "")
	// update setting will trigger another reconcile
	return c.settingClient.Update(settingCopy)
}

func (c *BridgeVLANController) OnRemove(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil {
		return nil, nil
	}

	if setting.Value == "" || key != NetworkSettingName {
		return setting, nil
	}

	klog.Info("harvester network setting has been deleted.")
	networkSetting, err := DecodeNetworkSettings(setting.Value)
	if err != nil {
		return nil, fmt.Errorf("decode failed, error: %w, value: %s", err, setting.Value)
	}

	nic, err := bridge.GetLink(networkSetting.NIC)
	if err != nil {
		return nil, err
	}

	// get nad vid list
	vidList, err := c.getNadVidList()
	if err != nil {
		return nil, fmt.Errorf("get nad vid list failed, error: %v", err)
	}

	klog.Infof("vid list: %+v", vidList)

	// if return address failed, log and continue
	if err := c.bridge.ReturnAddr(nic, vidList); err != nil {
		klog.Errorf("return address failed, error: %v, NIC: %s", err, networkSetting.NIC)
	}

	if err := c.bridge.Delete(); err != nil {
		klog.Errorf("delete bridge failed, error: %v, bridge: %+v", err, c.bridge)
	}

	settingCopy := setting.DeepCopy()
	harvesterv1.SettingConfigured.False(settingCopy)
	harvesterv1.SettingConfigured.Reason(settingCopy, "NIC is removed")
	if _, err := c.settingClient.Update(settingCopy); err != nil {
		return nil, err
	}

	return setting, nil
}

func (c *BridgeVLANController) configBridgeNetwork(setting *NetworkSetting) error {
	// ensure the bridge existed
	if err := c.bridge.Ensure(); err != nil {
		return fmt.Errorf("ensure bridge failed, error: %w, bridge: %+v", err, c.bridge)
	}

	// get nad vid list
	vidList, err := c.getNadVidList()
	if err != nil {
		return fmt.Errorf("get nad vid list failed, error: %v", err)
	}
	klog.Infof("vid list: %+v", vidList)

	nic, err := bridge.GetLink(setting.NIC)
	if err != nil {
		return err
	}

	configuredNIC, err := bridge.GetLink(setting.ConfiguredNIC)
	if err != nil {
		return err
	}

	// nic is nil if setting.NIC is an empty string
	// return addresses to configured nic if nic is empty
	if nic == nil && configuredNIC != nil {
		if err := c.bridge.ReturnAddr(configuredNIC, vidList); err != nil {
			klog.Errorf("return address failed, error: %v, NIC: %s", err, setting.ConfiguredNIC)
		}
		return nil
	}

	// Check whether the master of nic is bridge
	// - Yes，do nothing
	// - No, make bridge return addresses to configured nic, lend addresses to bridge, return.
	if nic.Attrs().MasterIndex != c.bridge.Index {
		if configuredNIC != nil {
			if err := c.bridge.ReturnAddr(configuredNIC, vidList); err != nil {
				klog.Errorf("return address failed, error: %v, NIC: %s", err, setting.ConfiguredNIC)
			}
		}

		// lend address to bridge
		if err := c.bridge.BorrowAddr(nic, vidList); err != nil {
			return fmt.Errorf("borrow address failed, error: %w, nic: %s", err, setting.NIC)
		}

		return nil
	}

	// Check whether bridge has at least one address
	// - Yes, do nothing
	// - No, borrow addresses from nic, return
	addrList, err := c.bridge.ListAddr()
	if err != nil {
		return fmt.Errorf("could not list addresses, error: %w, link: %s", err, c.bridge.Name)
	}
	if len(addrList) == 0 {
		if err := c.bridge.BorrowAddr(nic, vidList); err != nil {
			return fmt.Errorf("borrow address failed, error: %w, nic: %s", err, setting.NIC)
		}
	}

	return nil
}

func (c *BridgeVLANController) getNadVidList() ([]uint16, error) {
	nads, err := c.nadCache.List("", labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list nad failed, error: %v", err)
	}

	vidList := make([]uint16, 0)

	for _, nad := range nads {
		config, err := util.DecodeNetConf(nad.Spec.Config)
		if err != nil {
			klog.Errorf("failed to decode vlan config: %s", err.Error())
			continue
		}

		if config.Type == BridgeCNIName && config.BrName == BridgeName {
			klog.V(2).Infof("add nad:%s with vid:%d to the list", nad.Name, config.Vlan)
			vidList = append(vidList, uint16(config.Vlan))
		}
	}

	return vidList, nil
}
