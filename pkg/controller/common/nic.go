package common

import (
	"fmt"

	ctlharv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
)

const KeyHostName = "HOSTNAME"

func GetNIC(nic string, settingCache ctlharv1.SettingCache) (string, error) {
	if nic != "" {
		return nic, nil
	}

	return GetDefaultNIC(settingCache)
}

func GetDefaultNIC(settingCache ctlharv1.SettingCache) (string, error) {
	networkSetting, err := GetNetworkSetting(settingCache)
	if err != nil {
		return "", err
	}

	return networkSetting.NIC, nil
}

func GetNetworkSetting(settingCache ctlharv1.SettingCache) (*NetworkSetting, error) {
	setting, err := settingCache.Get(NetworkSettingName)
	if err != nil {
		return nil, fmt.Errorf("get vlan setting failed, error: %+v", err)
	}

	return DecodeNetworkSettings(setting.Value)
}
