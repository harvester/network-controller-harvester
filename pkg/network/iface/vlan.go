package iface

import (
	"errors"
	"fmt"

	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func (l *Link) GetVlanSubInterfaceAndOperState(vid uint16) (netlink.Link, bool, error) {
	linkName := utils.GetClusterNetworkBrVlanDevice(l.Attrs().Name, vid)
	// Check if link exists
	if vlanLink, err := netlink.LinkByName(linkName); err == nil {
		// Check if already UP
		if vlanLink.Attrs().OperState == netlink.OperUp {
			return vlanLink, true, nil
		} else {
			return vlanLink, false, nil
		}
	}

	return nil, false, fmt.Errorf("get vlan subinterface failed, link: %s, vid: %d", linkName, vid)
}

// CreateVlanSubInterface creates a new vlan sub-interface
// Equivalent to: `ip link add link <bridge-link-br> name <vlansubintf-name> type vlan id <vlan-id>.`
// `ip link set dev <vlansubintf-name> up`
func (l *Link) CreateVlanSubInterface(vid uint16) error {
	//check if vlan subinterface already exists
	if link, operUp, err := l.GetVlanSubInterfaceAndOperState(vid); err == nil {
		if operUp {
			return nil
		} else {
			if err := netlink.LinkSetUp(link); err != nil {
				return fmt.Errorf("set vlan subinterface up failed, error: %v, link: %s, vid: %d", err, link.Attrs().Name, vid)
			}
		}
	}

	linkName := utils.GetClusterNetworkBrVlanDevice(l.Attrs().Name, vid)

	vlan := &netlink.Vlan{
		LinkAttrs: netlink.LinkAttrs{
			Name:        linkName,
			ParentIndex: l.Attrs().Index,
		},
		VlanId: int(vid),
	}

	if err := netlink.LinkAdd(vlan); err != nil && !errors.Is(err, unix.EEXIST) {
		return fmt.Errorf("add vlan subinterface failed, error: %v, link: %s, vid: %d", err, vlan.Name, vid)
	}

	if err := netlink.LinkSetUp(vlan); err != nil {
		return fmt.Errorf("set vlan subinterface up failed, error: %v, link: %s, vid: %d", err, vlan.Attrs().Name, vid)
	}

	return nil
}

// DelVlanSubInterface deletes a vlan sub-interface
// Equivalent to: `ip link del dev <vlansubintf-name>`
func (l *Link) DelVlanSubInterface(vid uint16) error {
	linkName := utils.GetClusterNetworkBrVlanDevice(l.Attrs().Name, vid)
	vlanLink, err := netlink.LinkByName(linkName)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			return nil
		} else {
			return fmt.Errorf("finding the vlan subinterface failed, error: %v, link: %s, vid: %d", err, linkName, vid)
		}
	}

	if err := netlink.LinkDel(vlanLink); err != nil {
		return fmt.Errorf("delete vlan subinterface failed, error: %v, link: %s, vid: %d", err, linkName, vid)
	}

	return nil
}

func (l *Link) SetIPAddress(cidr string, vid uint16) error {
	linkName := utils.GetClusterNetworkBrVlanDevice(l.Attrs().Name, vid)
	vlanLink, err := netlink.LinkByName(linkName)
	if err != nil {
		return fmt.Errorf("finding vlan subinterface failed, error: %v, link: %s, vid: %d", err, linkName, vid)
	}

	ipAddr, err := netlink.ParseAddr(cidr)
	if err != nil {
		return err
	}

	if err := netlink.AddrReplace(vlanLink, ipAddr); err != nil {
		return fmt.Errorf("set ip address failed, error: %v, link: %s, ipNet: %v", err, l.Attrs().Name, ipAddr)
	}

	//delete other ip addresses (configured by previous DHCP lease or other static IPs)
	addresses, err := netlink.AddrList(vlanLink, netlink.FAMILY_V4)
	if err != nil {
		return err
	}

	for _, address := range addresses {
		if !ipAddr.IP.Equal(address.IP) {
			if err := netlink.AddrDel(vlanLink, &address); err != nil {
				return fmt.Errorf("delete ip address failed, error: %v, link: %s, ipNet: %v", err, l.Attrs().Name, address)
			}
		}
	}

	return nil
}
