package iface

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

var (
	ErrCleanupIPv4   = errors.New("failed to cleanup stale ipv4")
	ErrCleanupIPv6   = errors.New("failed to cleanup stale ipv6")
	ErrCleanupIPv4v6 = errors.New("failed to cleanup stale ipv4 ipv6")
)

// LeasedMultiFamilyIP wraps MultiFamilyIP with IPv6 lifetime metadata (if get from DHCP server)
// to ensure the kernel natively manages address expiration and deprecation.
// if lease is not set (defaults to 0), it means static IPv6, no lease times
type MultiFamilyIPWithLease struct {
	*networkv1.MultiFamilyIP
	V6PreferredLifetime time.Duration
	V6ValidLifetime     time.Duration
}

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

// SetIPAddress configures the given CIDR on the VLAN subinterface.
// It applies the new IP address before removing any stale IP addresses
// on the link.
// SetIPAddress is the legacy API for single-IPv4 management.
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

	// Clean up stale IPv4 addresses using the member helper
	if err := l.syncFamilyIPs(vlanLink, netlink.FAMILY_V4, ipAddr.IP, l.Attrs().Name); err != nil {
		return err
	}

	return nil
}

// syncFamilyIPs clears stale addresses of a given family, keeping only the expectedIP.
// It requires a non-nil expectedIP to execute; if expectedIP is nil, it skips cleanup entirely
// to protect unmanaged or other-family addresses during partial/single-family updates.
func (l *Link) syncFamilyIPs(vlanLink netlink.Link, family int, expectedIP net.IP, linkName string) error {
	// If no expected IP is provided for this family, skip syncing/cleaning up.
	if expectedIP == nil {
		return nil
	}

	// The local check is cheaper than a reconciler cycle; all system calls are expensive.
	isNotFound := func(err error) bool {
		errMsg := err.Error()
		// Check wrapped errors or standard os/syscall errors
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.ENODEV) {
			return true
		}
		return strings.Contains(errMsg, "no such process") ||
			strings.Contains(errMsg, "address not found") ||
			strings.Contains(errMsg, "does not exist")
	}

	addresses, err := netlink.AddrList(vlanLink, family)
	if err != nil {
		return fmt.Errorf("listing addresses for family %d failed: %v", family, err)
	}

	for _, address := range addresses {
		// CRITICAL: Never touch or delete IPv6 link-local addresses (fe80::/10) managed by the kernel
		if family == netlink.FAMILY_V6 && address.IP.IsLinkLocalUnicast() {
			continue
		}

		if expectedIP == nil || !expectedIP.Equal(address.IP) {
			if err := netlink.AddrDel(vlanLink, &address); err != nil {
				if isNotFound(err) {
					continue
				}
				return fmt.Errorf("delete stale ip address failed, error: %v, link: %s, ipNet: %v", err, linkName, address)
			}
		}
	}
	return nil
}

// SetMultiFamilyIPAddresses handles multi-family IP configurations.
func (l *Link) SetMultiFamilyIPAddresses(ips *MultiFamilyIPWithLease, vid uint16) error {
	if ips == nil {
		return fmt.Errorf("SetMultiFamilyIPAddresses: the input ips is nil")
	}
	linkName := utils.GetClusterNetworkBrVlanDevice(l.Attrs().Name, vid)
	vlanLink, err := netlink.LinkByName(linkName)
	if err != nil {
		return fmt.Errorf("finding vlan subinterface failed, error: %v, link: %s, vid: %d", err, linkName, vid)
	}

	// --- Step 1: Parse both IPs first and validate before any 'write' actions ---
	var desiredIPv4Addr, desiredIPv6Addr *netlink.Addr
	var desiredIPv4, desiredIPv6 net.IP

	if ips.IPv4 != nil && string(*ips.IPv4) != "" {
		ipv4Str := string(*ips.IPv4)
		ipv4Addr, err := netlink.ParseAddr(ipv4Str)
		if err != nil {
			return fmt.Errorf("invalid ipv4 cidr %q: %v", ipv4Str, err)
		}
		desiredIPv4Addr = ipv4Addr
		desiredIPv4 = ipv4Addr.IP
	}

	if ips.IPv6 != nil && string(*ips.IPv6) != "" {
		ipv6Str := string(*ips.IPv6)
		ipv6Addr, err := netlink.ParseAddr(ipv6Str)
		if err != nil {
			return fmt.Errorf("invalid ipv6 cidr %q: %v", ipv6Str, err)
		}
		// Apply DHCPv6 lifetimes directly using the v6 prefixed fields if it is leased IP
		if ips.V6PreferredLifetime > 0 {
			ipv6Addr.PreferedLft = int(ips.V6PreferredLifetime.Seconds())
		}
		if ips.V6ValidLifetime > 0 {
			ipv6Addr.ValidLft = int(ips.V6ValidLifetime.Seconds())
		}
		desiredIPv6Addr = ipv6Addr
		desiredIPv6 = ipv6Addr.IP
	}

	// --- Step 2: Apply valid IPv4 and IPv6 addresses ---
	// IPv4 is intentionally applied first to establish a predictable primary
	// address order on the interface for legacy or fallback-sensitive applications.
	if desiredIPv4Addr != nil {
		if err := netlink.AddrReplace(vlanLink, desiredIPv4Addr); err != nil {
			return fmt.Errorf("set ipv4 address failed, error: %v, link: %s, ipNet: %v", err, linkName, desiredIPv4Addr)
		}
	}

	if desiredIPv6Addr != nil {
		if err := netlink.AddrReplace(vlanLink, desiredIPv6Addr); err != nil {
			// Best-effort rollback: If IPv6 configuration fails after IPv4 was
			// successfully bound, attempt to remove the newly applied IPv4 address
			// to prevent leaving the interface in a partial, half-configured state.
			if desiredIPv4Addr != nil {
				if errDel := netlink.AddrDel(vlanLink, desiredIPv4Addr); errDel != nil {
					logrus.Errorf("setup ipv6 failed (%v); best-effort rollback of IPv4 also failed (%v) on link %s", err, errDel, linkName)
				}
			}

			return fmt.Errorf("set ipv6 address failed, error: %v, link: %s, ipNet: %v", err, linkName, desiredIPv6Addr)
		}
	}

	// --- Step 3: Clear stale addresses using the member helper (skipped automatically if the desired IP is nil/empty) ---
	// Note: If stale address cleanup fails, we intentionally avoid performing a complex
	// rollback of the newly applied lease. DHCP acquisition and netlink replacement are
	// expensive operations. Instead, we return a distinguishable cleanup error, allowing
	// the parent controller to re-queue the reconciliation loop and eventually achieve
	// convergence without discarding a valid, freshly acquired lease.
	if err := l.syncFamilyIPs(vlanLink, netlink.FAMILY_V4, desiredIPv4, l.Attrs().Name); err != nil {
		// book failure on cleanup dual
		if desiredIPv6Addr != nil {
			return fmt.Errorf("%w: %v", ErrCleanupIPv4v6, err)
		}
		// book failure on cleanup ipv4 only
		return fmt.Errorf("%w: %v", ErrCleanupIPv4, err)
	}

	if err := l.syncFamilyIPs(vlanLink, netlink.FAMILY_V6, desiredIPv6, l.Attrs().Name); err != nil {
		return fmt.Errorf("%w: %v", ErrCleanupIPv6, err)
	}

	return nil
}

func NewMultiFamilyIP() *MultiFamilyIPWithLease {
	return &MultiFamilyIPWithLease{
		MultiFamilyIP: &networkv1.MultiFamilyIP{},
	}
}

func NewMultiFamilyIPWithLeaseFromStaticIPs(ips *networkv1.MultiFamilyIP) *MultiFamilyIPWithLease {
	return &MultiFamilyIPWithLease{
		MultiFamilyIP: ips,
	}
}

func NewMultiFamilyIPWithLeaseFromIPv4(ipv4 *networkv1.IPAddr) *MultiFamilyIPWithLease {
	return &MultiFamilyIPWithLease{
		MultiFamilyIP: &networkv1.MultiFamilyIP{
			IPv4: ipv4,
		},
	}
}

func NewMultiFamilyIPWithLeaseFromIPv6(ipv6 *networkv1.IPAddr, prefLft, validLft time.Duration) *MultiFamilyIPWithLease {
	return &MultiFamilyIPWithLease{
		MultiFamilyIP: &networkv1.MultiFamilyIP{
			IPv6: ipv6,
		},
		V6PreferredLifetime: prefLft,
		V6ValidLifetime:     validLft,
	}
}
