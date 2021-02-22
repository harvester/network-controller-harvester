package common

import (
	"fmt"
	"os"

	ctlharv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"

	"github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
)

const KeyHostName = "HOSTNAME"

func GetNIC(hostNetworkCache v1alpha1.HostNetworkCache, settingCache ctlharv1.SettingCache) (string, error) {
	hn, err := hostNetworkCache.Get(HostNetworkNamespace, os.Getenv(KeyHostName))
	if err != nil {
		return "", err
	}

	if hn.Spec.NIC != "" {
		return hn.Spec.NIC, nil
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
