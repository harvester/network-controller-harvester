package iface

import (
	"fmt"
	"syscall"

	"github.com/vishvananda/netlink"

	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	BridgeSuffix         = "-br"
	bridgeNFCallIptables = "net/bridge/bridge-nf-call-iptables"

	lenOfBridgeSuffix = 3 // lengh of BridgeSuffix
)

type Bridge struct {
	*netlink.Bridge
}

func NewBridge(name string) *Bridge {
	vlanFiltering := true
	return &Bridge{
		&netlink.Bridge{
			LinkAttrs:     netlink.LinkAttrs{Name: name},
			VlanFiltering: &vlanFiltering,
		},
	}
}

// Ensure bridge
// set promiscuous mod default
func (br *Bridge) Ensure() error {
	if err := netlink.LinkAdd(br); err != nil && err != syscall.EEXIST {
		return fmt.Errorf("add iface failed, error: %w, iface: %v", err, br)
	}

	// Re-fetch link to read all attributes and if it's already existing,
	// ensure it's really a bridge with similar configuration
	if err := br.Fetch(); err != nil {
		return err
	}

	if br.Promisc != 1 {
		if err := netlink.SetPromiscOn(br); err != nil {
			return fmt.Errorf("set promiscuous mode failed, error: %w, iface: %v", err, br)
		}
	}

	if br.VlanFiltering != nil && *br.VlanFiltering == false {
		if err := netlink.BridgeSetVlanFiltering(br.Bridge, true); err != nil {
			return fmt.Errorf("set vlan filtering failed, error: %w, iface: %v", err, br)
		}
	}

	if br.OperState != netlink.OperUp {
		if err := netlink.LinkSetUp(br); err != nil {
			return err
		}
	}

	// Re-fetch bridge to ensure br.Bridge contains all latest attributes.
	return br.Fetch()
}

func DisableBridgeNF() error {
	return utils.EnsureSysctlValue(bridgeNFCallIptables, "0")
}

func (br *Bridge) Fetch() error {
	l, err := netlink.LinkByName(br.Name)
	if err != nil {
		return fmt.Errorf("could not lookup link %s, error: %w", br.Name, err)
	}

	b, ok := l.(*netlink.Bridge)
	if !ok {
		return fmt.Errorf("%s already exists but is not a bridge", br.Name)
	}

	br.Bridge = b

	return nil
}
