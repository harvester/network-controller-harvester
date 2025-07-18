package utils

import (
	"fmt"
)

const (
	MaxClusterNetworkNameLen = MaxDeviceNameLen - LenOfBridgeSuffix
)

func IsClusterNetworkNameValid(nm string) (bool, error) {
	if len(nm) > MaxClusterNetworkNameLen {
		return false, fmt.Errorf("the length of the clusterNetwork name %v is more than %d", nm, MaxClusterNetworkNameLen)
	}
	return true, nil
}
