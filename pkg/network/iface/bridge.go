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

	// Re-fetch link to read all attributes and if it already existed,
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
	tempBr, err = fetchByName(br.Name())
	if err != nil {
		return err
	}
	br.bridge = tempBr

	return nil
}

func (br *Bridge) Delete() error {
	if err := netlink.LinkDel(br.bridge); err != nil {
		return fmt.Errorf("could not delete link %s, error: %w", br.bridge.Name, err)
	}

	return nil
}

func (br *Bridge) configIPv4AddrFromSlave(slave *Link) error {
	slaveAddrList, err := netlink.AddrList(slave.link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("list IPv4 address of %s failed, error: %w", slave.link.Attrs().Name, err)
	}
	brAddrList, err := netlink.AddrList(br.bridge, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("list IPv4 address of %s failed, error: %w", br.bridge.Name, err)
	}
	delList := relativeComplement(slaveAddrList, brAddrList)
	addList := relativeComplement(brAddrList, slaveAddrList)
	for _, addr := range delList {
		klog.Infof("delete address: %+v", addr)
		if err := netlink.AddrDel(br.bridge, &addr); err != nil {
			return fmt.Errorf("could not add address, error: %w, link: %s, addr: %+v", err, br.bridge.Name, addr)
		}
	}

	for _, addr := range addList {
		addr.Label = br.bridge.Name
		klog.Infof("replace address: %+v", addr)
		if err := netlink.AddrReplace(br.bridge, &addr); err != nil {
			return fmt.Errorf("could not add address, error: %w, link: %s, addr: %+v", err, br.bridge.Name, addr)
		}
	}

	return nil
}

func (br *Bridge) replaceRoutes(slave *Link) error {
	if err := br.ToLink().ReplaceRoutes(slave); err != nil {
		return fmt.Errorf("replaces routes from %s to %s failed, error: %w", br.bridge.Name, slave.link.Attrs().Name, err)
	}

	return nil
}

func (br *Bridge) ConfigIPv4AddrFromSlave(slave *Link, routes []*netlink.Route) error {
	if err := br.configIPv4AddrFromSlave(slave); err != nil {
		return fmt.Errorf("configure IPv4 addresses from slave link %s failed, error: %w", slave.link.Attrs().Name, err)
	}

	addr, err := netlink.AddrList(br.bridge, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("list addr of bridge %s failed, error: %w", br.bridge.Name, err)
	}
	br.addr = addr

	if err = br.replaceRoutes(slave); err != nil {
		return fmt.Errorf("replace route rules of slave link %s failed, error: %w", slave.link.Attrs().Name, err)
	}

	// configure route rules passed by parameters
	// In case of resuming harvester-br0 deleted atypically, we get the original route rules from CR.
	// Moreover, we can reconfigure the special route rules configured by uses.
	for _, route := range routes {
		// the bridge index may have changed after reboot, that is the reason we correct it
		route.LinkIndex = br.Index()
		if err := netlink.RouteAdd(route); err != nil {
			klog.Warningf("could not add route, error: %s, route: %v", err.Error(), route)
		} else {
			klog.Infof("add route: %+v", route)
		}
	}

	return nil
}

func (br *Bridge) ClearAddr() error {
	addrList, err := netlink.AddrList(br.bridge, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("list IPv4 address of %s failed, error: %w", br.bridge.Name, err)
	}

	for _, addr := range addrList {
		if err := netlink.AddrDel(br.bridge, &addr); err != nil {
			return fmt.Errorf("delete address of %s failed, error: %w", br.bridge.Name, err)
		}
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
		return nil, fmt.Errorf("%s already exists but is not a iface", name)
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
