package bridge

import (
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"

	"github.com/rancher/harvester-network-controller/pkg/dhcp"
)

type Bridge struct {
	*netlink.Bridge

	dhcpSignal chan bool
	dhcpClient *dhcp.Client
}

func NewBridge(name string) *Bridge {
	vlanFiltering := true
	br := &Bridge{
		Bridge: &netlink.Bridge{
			LinkAttrs:     netlink.LinkAttrs{Name: name},
			VlanFiltering: &vlanFiltering,
		},
		dhcpSignal: make(chan bool),
		dhcpClient: dhcp.NewClient(name),
	}

	go br.StartDHCPClientDaemon()

	return br
}

// StartDHCPDaemon maintain dynamic ip address of bridge with dhcp
func (br *Bridge) StartDHCPClientDaemon() {
	klog.Info("start dhcp daemon")
	for signal := range br.dhcpSignal {
		if !signal && br.dhcpClient.IsRunning() {
			br.dhcpClient.Stop()
			continue
		}

		go br.dhcpClient.Start()
	}

	klog.Info("end dhcp daemon")
}

func (br *Bridge) DHCPHealthCheck() {
	addrList, err := br.ListAddr()
	if err != nil {
		klog.Errorf("could not list addresses, error: %s, link: %s", err.Error(), br.Name)
		return
	}

	if len(addrList) != 0 && !br.dhcpClient.IsRunning() {
		br.dhcpClient.Start()
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

	// Re-fetch bridge to ensure br.Bridge contains all latest attributes.
	br.Bridge, err = fetchByName(br.Name)
	if err != nil {
		return err
	}

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

	// stop DHCP client for physical nic with connman, otherwise, dhcp client will failed and configure one of IP
	// address in subnet 169.254.0.0/16. More deadly, the default IP route rule for physical nic is also changed.
	if err := lender.stopDHCP(); err != nil {
		return fmt.Errorf("stop dhcp client for %s failed, error: %w", lender.Attrs().Name, err)
	}
	// start bridge dhcp client
	br.dhcpSignal <- true

	if err := br.configAddr(lender); err != nil {
		return fmt.Errorf("configure IP address for %s failed, error: %w", br.Name, err)
	}

	return nil
}

func (br *Bridge) configAddr(src *Link) error {
	ip, mask, _, err := br.dhcpClient.GetIPv4Addr()
	if err != nil {
		return fmt.Errorf("%s get IPv4 address failed, error: %w", br.Name, err)
	}
	addr := &netlink.Addr{IPNet: &net.IPNet{IP: ip, Mask: mask}}
	dst := &Link{Link: br.Bridge}
	if err := netlink.AddrReplace(dst, addr); err != nil {
		return fmt.Errorf("could not add address, error: %w, link: %s", err, dst.Attrs().Name)
	}

	routeList, err := netlink.RouteList(src, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("could not list routes, error: %w, link: %s", err, src.Attrs().Name)
	}

	for i := range routeList {
		routeList[i].LinkIndex = dst.Attrs().Index
		if err := netlink.RouteReplace(&routeList[i]); err != nil {
			klog.Errorf("could not replace route, error: %s, route: %v", err.Error(), routeList[i])
			routeList[i].LinkIndex = src.Attrs().Index
			// remove route rule that can not be replaced, for example,
			// the auto-generated route `172.16.0.0/16 dev harvester-br0 proto kernel scope link src 172.16.0.76`
			if err := netlink.RouteDel(&routeList[i]); err != nil {
				klog.Errorf("could not delete route, error: %s, route: %v", err.Error(), routeList[i])
			}
		} else {
			klog.Infof("replace route, %+v", routeList[i])
		}
	}

	return nil
}

func (br *Bridge) delAddr() error {
	addrList, err := br.ListAddr()
	if err != nil {
		return fmt.Errorf("list IPv4 address of %s failed, error: %w", br.Name, err)
	}

	num := len(addrList)
	if num == 0 {
		return nil
	}
	if num > 1 {
		return fmt.Errorf("not support multiple addresses, iface: %s, address number: %d", br.Name, num)
	}

	if err := netlink.AddrDel(br, &addrList[0]); err != nil {
		return fmt.Errorf("delete address of %s failed, error: %w", br.Name, err)
	}

	return nil
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
	if err := returnee.setNoMaster(); err != nil {
		return fmt.Errorf("set no master for %s failed, error: %w", returnee.Attrs().Name, err)
	}

	// stop bridge dhcp client
	br.dhcpSignal <- false
	if err := br.delAddr(); err != nil {
		return err
	}
	// start dhcp client for physical nic
	if err := returnee.startDHCP(); err != nil {
		return fmt.Errorf("start dhcp client for link %s failed, error: %w", br.Name, err)
	}

	return nil
}

func (br *Bridge) ListAddr() ([]netlink.Addr, error) {
	return netlink.AddrList(br, netlink.FAMILY_V4)
}
