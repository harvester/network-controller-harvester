package utils

import (
	"github.com/vishvananda/netlink"
)

func StringToBondAdSelect(s string) netlink.BondAdSelect {
	adSelect, ok := netlink.StringToBondAdSelectMap[s]
	if !ok {
		return -1
	}
	return adSelect
}

func StringToBondArpValidate(s string) netlink.BondArpValidate {
	arpValidate, ok := netlink.StringToBondArpValidateMap[s]
	if !ok {
		return -1
	}
	return arpValidate
}

func StringToBondArpAllTargets(s string) netlink.BondArpAllTargets {
	arpAllTargets, ok := netlink.StringToBondArpAllTargetsMap[s]
	if !ok {
		return -1
	}
	return arpAllTargets
}
