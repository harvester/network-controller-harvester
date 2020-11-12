package network

import (
	"context"
	"fmt"

	harvesterv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	harvcontroller "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/apply"
	"k8s.io/klog"

	"github.com/rancher/harvester-network-controller/pkg/bridge"
)

type BridgeVLANController struct {
	settingClient harvcontroller.SettingClient
	settingsCache harvcontroller.SettingCache
	apply         apply.Apply
	bridge        *bridge.Bridge
}

const (
	Name                = "bridge-vlan-controller"
	networkSettingsName = "network-setting"
	BridgeName          = "harvester-br0"
)

func Register(ctx context.Context, apply apply.Apply, setting harvcontroller.SettingController) error {
	apply = apply.WithSetID(Name).WithCacheTypes(setting)

	controller := &BridgeVLANController{
		settingClient: setting,
		settingsCache: setting.Cache(),
		apply:         apply,
		bridge:        bridge.NewBridge(BridgeName),
	}

	if err := initNetworkSettings(controller.settingClient); err != nil {
		return err
	}

	setting.OnChange(ctx, Name, controller.OnChange)
	setting.OnRemove(ctx, Name, controller.OnRemove)
	return nil
}

func (c *BridgeVLANController) OnChange(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}
	if setting.Value == "" || key != networkSettingsName {
		return setting, nil
	}

	networkSetting, err := decodeNetworkSettings(setting.Value)
	if err != nil {
		return nil, fmt.Errorf("decode failed, error: %w, value: %s", err, setting.Value)
	}

	if err := c.configBridgeNetwork(networkSetting); err != nil {
		return nil, err
	}

	networkSetting.ConfiguredNIC = networkSetting.NIC
	settingCopy := setting.DeepCopy()
	settingCopy.Value, err = encodeNetworkSettings(networkSetting)
	if err != nil {
		return nil, fmt.Errorf("encode network settings failed, error: %w, networksettings: %+v", err, networkSetting)
	}
	// update setting will trigger another reconcile
	return c.settingClient.Update(settingCopy)
}

func (c *BridgeVLANController) OnRemove(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil {
		return nil, nil
	}
	if setting.Value == "" || key != networkSettingsName {
		return setting, nil
	}

	networkSetting, err := decodeNetworkSettings(setting.Value)
	if err != nil {
		return nil, fmt.Errorf("decode failed, error: %w, value: %s", err, setting.Value)
	}

	nic, err := bridge.GetLink(networkSetting.NIC)
	if err != nil {
		klog.Error(err)
		return setting, nil
	}

	// if return address failed, log and continue
	if err := c.bridge.ReturnAddr(nic); err != nil {
		klog.Errorf("return address failed, error: %v, NIC: %s", err, networkSetting.NIC)
	}

	if err := c.bridge.Delete(); err != nil {
		klog.Errorf("delete bridge failed, error: %v, bridge: %+v", err, c.bridge)
	}

	return setting, nil
}

func (c *BridgeVLANController) configBridgeNetwork(setting *NetworkSetting) error {
	nic, err := bridge.GetLink(setting.NIC)
	if err != nil {
		return err
	}
	configuredNIC, err := bridge.GetLink(setting.ConfiguredNIC)
	if err != nil {
		return err
	}

	// ensure bridge existed
	if err := c.bridge.Ensure(); err != nil {
		return fmt.Errorf("ensure bridge failed, error: %w, bridge: %+v", err, c.bridge)
	}

	// nic is nil if setting.NIC is an empty string
	// return addresses to configured nic if nic is empty
	if nic == nil {
		if err := c.bridge.ReturnAddr(configuredNIC); err != nil {
			klog.Errorf("return address failed, error: %v, NIC: %s", err, setting.ConfiguredNIC)
		}
		return nil
	}

	// Check whether the master of nic is bridge
	// - Yes，do nothing
	// - No, make bridge return addresses to configured nic, lend addresses to bridge, return.
	if nic.Attrs().MasterIndex != c.bridge.Index {
		if err := c.bridge.ReturnAddr(configuredNIC); err != nil {
			klog.Errorf("Return address failed, error: %v, NIC: %s", err, setting.ConfiguredNIC)
		}

		// lend address to bridge
		if err := c.bridge.BorrowAddr(nic); err != nil {
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
		if err := c.bridge.BorrowAddr(nic); err != nil {
			return fmt.Errorf("borrow address failed, error: %w, nic: %s", err, setting.NIC)
		}
	}

	return nil
}
