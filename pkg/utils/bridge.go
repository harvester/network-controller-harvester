package utils

import (
	"fmt"
	"strings"
)

const (
	BridgeSuffix       = "-br"
	BondSuffix         = "-bo"
	DefaultValueMiimon = 100

	LenOfBridgeSuffix = 3 // length of BridgeSuffix
	LenOfBondSuffix   = 3 // length of BondSuffix

	MaxDeviceNameLen = 15

	VlanSubInterfaceSpliter = "."

	// format: e.g. mgmt-br.2025
	ManagementClusterNetworkDevicePrefix = ManagementClusterNetworkName + BridgeSuffix + VlanSubInterfaceSpliter
)

func HasMgmtClusterNetworkDevicePrefix(link string) bool {
	return strings.HasPrefix(link, ManagementClusterNetworkDevicePrefix)
}

// e.g. cn2-br.2025 is a valid vlan sub interface, and the device prefix is `cn2-br.`
func GetClusterNetworkDevicePrefix(cnName string) string {
	return fmt.Sprintf("%s%s%s", cnName, BridgeSuffix, VlanSubInterfaceSpliter)
}

func HasClusterNetworkDevicePrefix(link, prefix string) bool {
	return strings.HasPrefix(link, prefix)
}

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

func generateName(prefix, suffix string, lenOfSufix int) string {
	maxPrefixLen := MaxDeviceNameLen - lenOfSufix
	if len(prefix) > maxPrefixLen {
		return prefix[:maxPrefixLen] + suffix
	}
	return prefix + suffix
}

func GenerateBridgeName(prefix string) string {
	return generateName(prefix, BridgeSuffix, LenOfBridgeSuffix)
}

func GenerateBondName(prefix string) string {
	return generateName(prefix, BondSuffix, LenOfBondSuffix)
}
