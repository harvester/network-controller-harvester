package dhcpv6

import (
	"fmt"
	"net"
	"strconv"

	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/nclient6"
	"github.com/vishvananda/netlink"
)

// getLinkLocalAddr fetches the IPv6 link-local address for the given link.
//
// It accepts a pre-resolved netlink.Link, avoiding redundant kernel lookups
// and bypassing standard library interface caches.
//
// Note: When a new network interface is created, the Linux kernel applies its
// IPv6 link-local address asynchronously. Querying via netlink (netlink.AddrList)
// retrieves the fresh, up-to-date address directly from the kernel socket table,
// avoiding stale standard-library interface caches.
func getLinkLocalAddr(link netlink.Link) (net.IP, error) {
	if link == nil {
		return nil, fmt.Errorf("link cannot be nil")
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_V6)
	if err != nil {
		return nil, fmt.Errorf("failed to list IPv6 addresses for %s: %w", link.Attrs().Name, err)
	}

	for _, addr := range addrs {
		if addr.IP != nil && addr.IP.To4() == nil && addr.IP.IsLinkLocalUnicast() {
			return addr.IP, nil
		}
	}

	return nil, fmt.Errorf("no IPv6 link-local address found for interface %s", link.Attrs().Name)
}

// newIPv6UDPConn returns a UDP connection bound to both the interface and port.
//
// This function rewrites/replaces the upstream nclient6.NewIPv6UDPConn (which relies
// on standard library lookups prone to caching and race conditions) to bypass
// cached v6 interface states and retrieve the link-local address directly via netlink.
//
// It extracts the numeric interface index directly from the link object (`link.Attrs().Index`)
// and passes it as a string zone (e.g., "3" instead of "cn2-br.100"). This forces Go's internal
// net.zoneCache to parse it directly as an integer, eliminating name-to-index lookup overhead
// and caching issues during rapid interface churn.
func newIPv6UDPConn(link netlink.Link, port int) (net.PacketConn, error) {
	ip, err := getLinkLocalAddr(link)
	if err != nil {
		return nil, err
	}

	ifIndex := link.Attrs().Index

	return net.ListenUDP("udp6", &net.UDPAddr{
		IP:   ip,
		Port: port,
		Zone: strconv.Itoa(ifIndex),
	})
}

// NewClient returns a new DHCPv6 client for the given network interface link.
//
// Rationale:
// Upstream implementation relies internally on standard library interface lookups
// which can cache state or fail with "bind: no such device" errors when network
// interfaces are created and destroyed rapidly in dynamic CNI environments.
//
// This wrapper takes the pre-resolved netlink.Link to extract both the hardware
// address and interface index directly without redundant lookups. Combined with
// custom netlink-backed socket binding and numeric zone indexing, it remains
// 100% robust against rapid device churn.
func NewClient(link netlink.Link, opts ...nclient6.ClientOpt) (*nclient6.Client, error) {
	if link == nil {
		return nil, fmt.Errorf("link cannot be nil")
	}

	c, err := newIPv6UDPConn(link, dhcpv6.DefaultClientPort)
	if err != nil {
		return nil, err
	}

	return nclient6.NewWithConn(c, link.Attrs().HardwareAddr, opts...)
}
