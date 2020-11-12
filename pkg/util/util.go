package util

import (
	"encoding/json"
	"fmt"

	harvesterv1 "github.com/rancher/harvester/pkg/api/network"
)

func DecodeNetConf(config string) (*harvesterv1.NetConf, error) {
	netconf := &harvesterv1.NetConf{}
	if err := json.Unmarshal([]byte(config), netconf); err != nil {
		return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, config)
	}

	return netconf, nil
}
