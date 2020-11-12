package bridge

import (
	"fmt"
	"syscall"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"
)

type Bridge struct {
	*netlink.Bridge
}

func NewBridge(name string) *Bridge {
	vlanFiltering := true
	return &Bridge{
		Bridge: &netlink.Bridge{
			LinkAttrs:     netlink.LinkAttrs{Name: name},
			VlanFiltering: &vlanFiltering,
		},
	}
}

// Ensure bridge
// set promiscuous mod default
func (br *Bridge) Ensure() error {
	if err := netlink.LinkAdd(br.Bridge); err != nil && err != syscall.EEXIST {
		return fmt.Errorf("add bridge failed, error: %w, bridge: %v", err, br)
	}

	// Re-fetch link to read all attributes and if it already existed,
	// ensure it's really a bridge with similar configuration
	tempBr, err := fetchByName(br.Name)
	if err != nil {
		return err
	}

	if tempBr.Promisc != 1 {
		if err := netlink.SetPromiscOn(br); err != nil {
			return fmt.Errorf("set promiscuous mode failed, error: %w, bridge: %v", err, br)
		}
	}

	if tempBr.OperState != netlink.OperUp {
		if err := netlink.LinkSetUp(br); err != nil {
			return err
		}
	}

	// TODO ensure vlan filtering

	return nil
}

func (br *Bridge) Delete() error {
	if err := netlink.LinkDel(br); err != nil {
		return fmt.Errorf("could not delete link %s, error: %w", br.Name, err)
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

// BorrowAddr borrow address from the specified host nic.
// Only support ipv4 address at this stage.
func (br *Bridge) BorrowAddr(lender *Link, vidList []uint16) error {
	if lender == nil {
		return fmt.Errorf("lender link could not be nil")
	}

	// Set bridge as master of lender nic
	if err := netlink.LinkSetMaster(lender, br); err != nil {
		return fmt.Errorf("could not set master, link: %s, master: %s", lender.Attrs().Name, br.Name)
	}

	// add bridge vlan
	for _, vid := range vidList {
		if err := lender.AddBridgeVlan(vid); err != nil {
			return err
		}
	}

	// transfer address
	return transferAddr(lender, &Link{Link: br.Bridge})
}

// ReturnAddr return address to returnee link
func (br *Bridge) ReturnAddr(returnee *Link, vidList []uint16) error {
	if returnee == nil {
		return fmt.Errorf("returnee link could not be nil")
	}

	// del bridge vlan
	for _, vid := range vidList {
		if err := returnee.DelBridgeVlan(vid); err != nil {
			return err
		}
	}

	// returnee nic set no master
	if err := netlink.LinkSetNoMaster(returnee); err != nil {
		return fmt.Errorf("could not set no master, error: %w, link: %s", err, returnee.Attrs().Name)
	}

	// transfer address
	if err := transferAddr(&Link{Link: br.Bridge}, returnee); err != nil {
		return fmt.Errorf("transfer address failed, error: %w, src link: %s, dst link: %s", err, br.Name, returnee.Attrs().Name)
	}

	return nil
}

func transferAddr(src, dst *Link) error {
	//  delete address of src link
	addrList, err := netlink.AddrList(src, netlink.FAMILY_V4)
	if err != nil || len(addrList) == 0 {
		return fmt.Errorf("could not get address or nic does not own an address, error: %w, link: %s", err, src.Attrs().Name)
	}

	// get routes of src link before deleting addr, cause routes will disappear if the address of link is deleted.
	routeList, err := netlink.RouteList(src, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("could not list routes, error: %w, link: %s", err, src.Attrs().Name)
	}

	for _, addr := range addrList {
		klog.Infof("del src addr, src: %s, addr: %+v", src.Attrs().Name, addr)
		if err := netlink.AddrDel(src, &addr); err != nil {
			return fmt.Errorf("could not delete address, error: %w, link:%s", err, src.Attrs().Name)
		}

		// replace address of dst link
		klog.Infof("add dst addr, dst: %s, addr: %+v", src.Attrs().Name, addr)
		addr.Label = dst.Attrs().Name
		if err := netlink.AddrReplace(dst, &addr); err != nil {
			return fmt.Errorf("could not add address, error: %w, link: %s", err, dst.Attrs().Name)
		}
	}

	// replace routes
	for i := range routeList {
		routeList[i].LinkIndex = dst.Attrs().Index
		if err := netlink.RouteReplace(&routeList[i]); err != nil {
			return fmt.Errorf("could not replace route, error: %w, route: %v", err, routeList[i])
		}
		klog.Infof("replace route, %+v", routeList[i])
	}

	return nil
}

func (br *Bridge) ListAddr() ([]netlink.Addr, error) {
	return netlink.AddrList(br, netlink.FAMILY_V4)
}
