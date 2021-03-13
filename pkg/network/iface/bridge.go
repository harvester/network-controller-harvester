package iface

import (
	"fmt"
	"syscall"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"
)

type Bridge struct {
	bridge *netlink.Bridge
	addr   []netlink.Addr
	routes []netlink.Route
}

func NewBridge(name string) *Bridge {
	vlanFiltering := true
	return &Bridge{
		bridge: &netlink.Bridge{
			LinkAttrs:     netlink.LinkAttrs{Name: name},
			VlanFiltering: &vlanFiltering,
		},
	}
}

// Ensure bridge
// set promiscuous mod default
func (br *Bridge) Ensure() error {
	if err := netlink.LinkAdd(br.bridge); err != nil && err != syscall.EEXIST {
		return fmt.Errorf("add iface failed, error: %w, iface: %v", err, br)
	}

	// Re-fetch link to read all attributes and if it's already existing,
	// ensure it's really a bridge with similar configuration
	tempBr, err := fetchByName(br.bridge.Name)
	if err != nil {
		return err
	}

	if tempBr.Promisc != 1 {
		if err := netlink.SetPromiscOn(br.bridge); err != nil {
			return fmt.Errorf("set promiscuous mode failed, error: %w, iface: %v", err, br)
		}
	}

	if tempBr.OperState != netlink.OperUp {
		if err := netlink.LinkSetUp(br.bridge); err != nil {
			return err
		}
	}
	// TODO ensure vlan filtering

	// Re-fetch bridge to ensure br.Bridge contains all latest attributes.
	return br.Fetch()
}

// Keep the bridge's IPv4 addresses are the same with the slave
func (br *Bridge) SyncIPv4Addr(slave IFace) error {
	delList := relativeComplement(slave.Addr(), br.addr)
	addList := relativeComplement(br.addr, slave.Addr())
	for _, addr := range delList {
		if err := netlink.AddrDel(br.bridge, &addr); err != nil {
			return fmt.Errorf("could not add address, error: %w, link: %s, addr: %+v", err, br.bridge.Name, addr)
		}
		klog.Infof("delete IPv4 address %+v", addr)
	}

	for _, addr := range addList {
		addr.Label = br.bridge.Name
		if err := netlink.AddrAdd(br.bridge, &addr); err != nil {
			return fmt.Errorf("could not add address, error: %w, link: %s, addr: %+v", err, br.bridge.Name, addr)
		}
		klog.Infof("add IPv4 address %+v", addr)
	}

	return nil
}

func (br *Bridge) ClearAddr() error {
	for _, addr := range br.addr {
		if err := netlink.AddrDel(br.bridge, &addr); err != nil {
			return fmt.Errorf("delete address of %s failed, error: %w", br.bridge.Name, err)
		}
		klog.Infof("delete IPv4 address %+v", addr)
	}

	return nil
}

func (br *Bridge) ToLink() *Link {
	return &Link{
		link:   br.bridge,
		addr:   br.addr,
		routes: br.routes,
	}
}

func (br *Bridge) Name() string {
	return br.bridge.Name
}

func (br *Bridge) Index() int {
	if br.bridge == nil {
		return 0
	}
	return br.bridge.Index
}

func (br *Bridge) Type() string {
	if br.bridge == nil {
		return "bridge"
	}
	return br.bridge.Type()
}

func (br *Bridge) LinkAttrs() *netlink.LinkAttrs {
	if br.bridge == nil {
		return nil
	}
	return &br.bridge.LinkAttrs
}

func (br *Bridge) Addr() []netlink.Addr {
	return br.addr
}

func (br *Bridge) Routes() []netlink.Route {
	return br.routes
}

func (br *Bridge) Fetch() error {
	tempBr, err := fetchByName(br.Name())
	if err != nil {
		return fmt.Errorf("fetch bridge %s failed, error: %w", br.Name(), err)
	}

	br.bridge = tempBr

	if br.addr, err = netlink.AddrList(br.bridge, netlink.FAMILY_V4); err != nil {
		return fmt.Errorf("refresh addresses of link %s failed, error: %w", br.Name(), err)
	}
	if br.routes, err = netlink.RouteList(br.bridge, netlink.FAMILY_V4); err != nil {
		return fmt.Errorf("refresh routes of link %s failed, error: %w", br.Name(), err)
	}

	return nil
}

func fetchByName(name string) (*netlink.Bridge, error) {
	l, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("could not lookup link %s, error: %w", name, err)
	}

	br, ok := l.(*netlink.Bridge)
	if !ok {
		return nil, fmt.Errorf("%s already exists but is not a bridge", name)
	}

	return br, nil
}

// find the relative complement of A (left side) in B (right side)
func relativeComplement(A, B []netlink.Addr) []netlink.Addr {
	complement := []netlink.Addr{}
	for i := range B {
		flag := true
		for j := range A {
			if B[i].Equal(A[j]) {
				flag = false
				break
			}
		}
		if flag {
			complement = append(complement, B[i])
		}
	}

	return complement
}
