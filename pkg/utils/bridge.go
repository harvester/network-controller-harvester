package utils

import (
	"fmt"
	"strings"
)

const (
	BridgeSuffix      = "-br"
	LenOfBridgeSuffix = 3 // lengh of BridgeSuffix
)

func IsBridgeNameValid(nm string) (bool, error) {
	if len(nm) <= LenOfBridgeSuffix {
		return false, fmt.Errorf("The length of bridge name %v is less than %v", nm, LenOfBridgeSuffix)
	}
	if !strings.HasSuffix(nm, BridgeSuffix) {
		return false, fmt.Errorf("The bridge name %v does not include suffix %v", nm, BridgeSuffix)
	}
	return true, nil
}

func GetClusterNetworkFromBridgeName(nm string) (string, error) {
	if _, err := IsBridgeNameValid(nm); err != nil {
		return "", err
	}
	return nm[:len(nm)-LenOfBridgeSuffix], nil
}
