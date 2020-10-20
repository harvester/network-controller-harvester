package network

import (
	"context"

	harvesterv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	harvcontroller "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/rancher/wrangler/pkg/apply"
)

type networkSetting struct {
	NIC           string
	ConfiguredNIC string
}

type BridgeVLANController struct {
	namespace     string
	settingClient harvcontroller.SettingClient
	settingsCache harvcontroller.SettingCache
	apply         apply.Apply
}

const (
	Name = "bridge-vlan-controller"
)

func Register(ctx context.Context, apply apply.Apply, setting harvcontroller.SettingController) {
	apply = apply.WithSetID(Name).WithCacheTypes(setting)

	controller := &BridgeVLANController{
		settingClient: setting,
		settingsCache: setting.Cache(),
		apply:         apply,
	}

	setting.OnChange(ctx, Name, controller.OnChange)
}

func (c *BridgeVLANController) OnChange(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil {
		return nil, nil
	}
	if setting.Value == "" {
		return setting, nil
	}
	// TODO, config/re-config the bridge

	// 1. convert setting to networSetting struct, return error if is invalid

	// 2. if has configured-bridge, reset the bridge and configured NIC first

	// 3. set the new configured NIC to the bridge

	// 4. update the status/log of the setting

	return setting, nil
}

func (c *BridgeVLANController) OnRemove(key string, setting *harvesterv1.Setting) (*harvesterv1.Setting, error) {
	if setting == nil {
		return nil, nil
	}
	if setting.Value == "" {
		return setting, nil
	}

	// TODO, bridge VLAN setting is not suppose to be removed, therefore we can add a backup setting on delete?

	return setting, nil
}
