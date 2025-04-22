package iface

import (
	"fmt"
)

const (
	MaxDeviceNameLen = 15
)

func GenerateName(prefix, suffix string) string {
	maxPrefixLen := MaxDeviceNameLen - len(suffix)
	if len(prefix) > maxPrefixLen {
		return prefix[:maxPrefixLen] + suffix
	}
	return prefix + suffix
}

// check if the bridge name is valid
func CheckBridgeName(brName string) error {
	_, err := GetBridgeNamePrefix(brName)
	return err
}

// get the bridge name exclude the suffix
func GetBridgeNamePrefix(brName string) (string, error) {
	lenOfBrName := len(brName)

	if lenOfBrName > MaxDeviceNameLen {
		return "", fmt.Errorf("the length of the brName can't be more than %v", MaxDeviceNameLen)
	}

	if lenOfBrName <= lenOfBridgeSuffix || brName[lenOfBrName-lenOfBridgeSuffix:] != BridgeSuffix {
		return "", fmt.Errorf("the suffix of the brName should be %s", BridgeSuffix)
	}

	return brName[:lenOfBrName-lenOfBridgeSuffix], nil
}
