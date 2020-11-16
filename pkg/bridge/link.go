package bridge

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

const defaultPVID = uint16(1)

type Link struct {
	netlink.Link
}

// GetLink by name
func GetLink(name string) (*Link, error) {
	if name == "" {
		return nil, nil
	}

	l, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("could not lookup link, error: %w, link: %s", err, name)
	}

	return &Link{Link: l}, nil
}

// AddBridgeVlan adds a new vlan filter entry
// Equivalent to: `bridge vlan add dev DEV vid VID master`
func (l *Link) AddBridgeVlan(vid uint16) error {
	if vid == defaultPVID {
		return nil
	}

	if err := netlink.BridgeVlanAdd(l.Link, vid, false, false, false, true); err != nil {
		return fmt.Errorf("add bridge vlan failed, error: %v, link: %s, vid: %d", err, l.Attrs().Name, vid)
	}

	return nil
}

// DelBridgeVlan adds a new vlan filter entry
// Equivalent to: `bridge vlan del dev DEV vid VID master`
func (l *Link) DelBridgeVlan(vid uint16) error {
	if vid == defaultPVID {
		return nil
	}

	if err := netlink.BridgeVlanDel(l.Link, vid, false, false, false, true); err != nil {
		return fmt.Errorf("delete bridge vlan failed, error: %v, link: %s, vid: %d", err, l.Attrs().Name, vid)
	}

	return nil
}
