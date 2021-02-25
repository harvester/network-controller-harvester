package common

import (
	"encoding/json"
	"fmt"

	harvnetwork "github.com/rancher/harvester/pkg/api/network"
)

var (
	NetworkSettingName = "network-setting"
)

type NetworkSetting struct {
	Type string

	// physical NIC(network interface card)
	NIC string

	// previous configured NIC
	ConfiguredNIC string
}

func DecodeNetworkSettings(value string) (*NetworkSetting, error) {
	setting := &NetworkSetting{}
	if err := json.Unmarshal([]byte(value), setting); err != nil {
		return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, value)
	}

	return setting, nil
}

func DecodeNetConf(config string) (*harvnetwork.NetConf, error) {
	netconf := &harvnetwork.NetConf{}
	if err := json.Unmarshal([]byte(config), netconf); err != nil {
		return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, config)
	}

	return netconf, nil
}
