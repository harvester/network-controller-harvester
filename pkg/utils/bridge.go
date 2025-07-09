package utils

import (
	"fmt"
	"strings"
)

const (
	BridgeSuffix = "-br"
	BondSuffix   = "-bo"

	LenOfBridgeSuffix = 3 // length of BridgeSuffix
	LenOfBondSuffix   = 3 // length of BondSuffix

	MaxDeviceNameLen         = 15
	MaxClusterNetworkNameLen = MaxDeviceNameLen - LenOfBridgeSuffix

	ManagementClusterNetworkDevicePrefix = ManagementClusterNetworkName + "-" + BridgeSuffix + "."
)

func IsBridgeNameValid(brName string) (bool, error) {
	lenOfBrName := len(brName)
	if lenOfBrName <= LenOfBridgeSuffix {
		return false, fmt.Errorf("the length of bridge name %v is less than %v", brName, LenOfBridgeSuffix)
	}
	if lenOfBrName > MaxDeviceNameLen {
		return false, fmt.Errorf("the length of the bridge name %v can't be more than %v", brName, MaxDeviceNameLen)
	}
	if !strings.HasSuffix(brName, BridgeSuffix) {
		return false, fmt.Errorf("the bridge name %v does not include suffix %v", brName, BridgeSuffix)
	}
	return true, nil
}

func GetClusterNetworkFromBridgeName(brName string) (string, error) {
	if _, err := IsBridgeNameValid(brName); err != nil {
		return "", err
	}
	return brName[:len(brName)-LenOfBridgeSuffix], nil
}

// check if the bridge name is valid
func CheckBridgeName(brName string) error {
	_, err := IsBridgeNameValid(brName)
	return err
}

// get the bridge name exclude the suffix
func GetBridgeNamePrefix(brName string) (string, error) {
	return GetClusterNetworkFromBridgeName(brName)
}

func GenerateBridgeName(prefix string) string {
	maxPrefixLen := MaxDeviceNameLen - LenOfBridgeSuffix
	if len(prefix) > maxPrefixLen {
		return prefix[:maxPrefixLen] + BridgeSuffix
	}
	return prefix + BridgeSuffix
}
