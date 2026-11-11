package iface

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/containernetworking/plugins/pkg/netlinksafe"
	"github.com/coreos/go-iptables/iptables"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	TypeLoopback = "loopback"
	TypeDevice   = "device"
	TypeBond     = "bond"

	ipv4Forward = "net/ipv4/ip_forward"

	tableFilter  = "filter"
	chainForward = "FORWARD"

	defaultPVID = uint16(utils.DefaultVlanID)
	minVlanID   = uint16(utils.MinVlanID)
)

type Link struct {
	netlink.Link
}

func NewLink(l netlink.Link) *Link {
	return &Link{
		Link: l,
	}
}

// AddBridgeVlan adds a new vlan filter entry
// Equivalent to: `bridge vlan add dev DEV vid VID master`
func (l *Link) AddBridgeVlan(vid uint16) error {
	// The command to configure PVID is not `bridge vlan add dev DEV vid VID master`
	if vid == defaultPVID || vid == minVlanID {
		return nil
	}

	if err := netlink.BridgeVlanAdd(l, vid, false, false, false, true); err != nil {
		return fmt.Errorf("add iface vlan failed, error: %v, link: %s, vid: %d", err, l.Attrs().Name, vid)
	}

	return nil
}

// DelBridgeVlan dels the vlan filter entry
// Equivalent to: `bridge vlan del dev DEV vid VID master`
func (l *Link) DelBridgeVlan(vid uint16) error {
	if vid == defaultPVID || vid == minVlanID {
		return nil
	}

	if err := netlink.BridgeVlanDel(l, vid, false, false, false, true); err != nil {
		return fmt.Errorf("delete iface vlan failed, error: %v, link: %s, vid: %d", err, l.Attrs().Name, vid)
	}

	return nil
}

// AddBridgeVlanSelf adds a new vlan filter entry to -br interface
// Equivalent to: `bridge vlan add dev DEV vid VID self`
func (l *Link) AddBridgeVlanSelf(vid uint16) error {
	// The command to configure PVID is not `bridge vlan add dev DEV vid VID master`
	if vid == defaultPVID || vid == minVlanID {
		return nil
	}

	if err := netlink.BridgeVlanAdd(l, vid, false, false, true, false); err != nil {
		return fmt.Errorf("add iface vlan failed, error: %v, link: %s, vid: %d", err, l.Attrs().Name, vid)
	}

	return nil
}

// DelBridgeVlan deletes a vlan filter entry from -br interface
// Equivalent to: `bridge vlan del dev DEV vid VID self`
func (l *Link) DelBridgeVlanSelf(vid uint16) error {
	if vid == defaultPVID || vid == minVlanID {
		return nil
	}

	if err := netlink.BridgeVlanDel(l, vid, false, false, true, false); err != nil {
		return fmt.Errorf("delete iface vlan failed, error: %v, link: %s, vid: %d", err, l.Attrs().Name, vid)
	}

	return nil
}

func (l *Link) ListBridgeVlan() ([]uint16, error) {
	m, err := netlink.BridgeVlanList()
	if err != nil {
		return nil, err
	}

	vlanInfo, ok := m[int32(l.Attrs().Index)] //nolint:gosec
	if !ok {
		return nil, nil
	}

	vids := make([]uint16, len(vlanInfo))
	for i := range vlanInfo {
		vids[i] = vlanInfo[i].Vid
	}

	return vids, nil
}

func (l *Link) ToVlanIDSet() (*utils.VlanIDSet, error) {
	m, err := netlink.BridgeVlanList()
	if err != nil {
		return nil, err
	}

	vlanInfo, ok := m[int32(l.Attrs().Index)] //nolint:gosec
	if !ok {
		return nil, nil
	}

	vis := utils.NewVlanIDSet()

	for i := range vlanInfo {
		if err := vis.SetUint16VID(vlanInfo[i].Vid); err != nil {
			return nil, fmt.Errorf("failed to set link %v vid %v to vlanset, error %w", l.Attrs().Name, vlanInfo[i].Vid, err)
		}
	}

	return vis, nil
}

// clearMacvlan to delete all the macvlan interfaces whose parent index equals l.Index()
func (l *Link) clearMacVlan() error {
	links, err := netlinksafe.LinkList()
	if err != nil {
		return err
	}
	for _, link := range links {
		if link.Attrs().ParentIndex == l.Attrs().Index && link.Type() == "macvlan" {
			if err := netlink.LinkDel(link); err != nil {
				return err
			}
			logrus.Infof("delete macvlan interface %s", link.Attrs().Name)
		}
	}

	return nil
}

func (l *Link) SetMaster(br *Bridge) error {
	if l.Attrs().MasterIndex == br.Index {
		return nil
	}

	if err := l.clearMacVlan(); err != nil {
		return err
	}
	if err := netlink.LinkSetMaster(l, br); err != nil {
		return fmt.Errorf("%s set %s as master failed, error: %w", l.Attrs().Name, br.Name, err)
	}

	return nil
}

func (l *Link) SetNoMaster() error {
	if l.Attrs().MasterIndex == 0 {
		return nil
	}

	logrus.Infof("%s set no master", l.Attrs().Name)

	return netlink.LinkSetNoMaster(l)
}

func (l *Link) EnsureIptForward() error {
	// enable ipv4Forward
	if err := utils.EnsureSysctlValue(ipv4Forward, "1"); err != nil {
		return fmt.Errorf("enable ipv4 forward failed, error: %w", err)
	}

	ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
	if err != nil {
		return err
	}
	rules, err := ipt.List(tableFilter, chainForward)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		if strings.HasPrefix(rule, "-P "+chainForward) {
			if strings.Fields(rule)[2] == "ACCEPT" {
				return nil
			}
			break
		}
	}

	if err := ipt.AppendUnique(tableFilter, chainForward, "-i", l.Attrs().Name, "-j", "ACCEPT"); err != nil {
		return err
	}
	return ipt.AppendUnique(tableFilter, chainForward, "-o", l.Attrs().Name, "-j", "ACCEPT")
}

func (l *Link) DeleteIptForward() error {
	ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
	if err != nil {
		return err
	}

	if err := ipt.DeleteIfExists(tableFilter, chainForward, "-i", l.Attrs().Name, "-j", "ACCEPT"); err != nil {
		return err
	}

	return ipt.DeleteIfExists(tableFilter, chainForward, "-o", l.Attrs().Name, "-j", "ACCEPT")
}

func (l *Link) Fetch() error {
	link, err := netlink.LinkByName(l.Attrs().Name)
	if err != nil {
		return fmt.Errorf("refresh link %s failed, error: %w", l.Attrs().Name, err)
	}

	l.Link = link

	return nil
}

func ListLinks(typeSelector map[string]bool) ([]*Link, error) {
	links, err := netlinksafe.LinkList()
	if err != nil {
		return nil, err
	}

	var linkList []*Link

	for _, link := range links {
		// filter loopback interface
		if link.Attrs().EncapType == TypeLoopback {
			continue
		}
		if typeSelector[link.Type()] {
			linkList = append(linkList, &Link{Link: link})
		}
	}

	return linkList, nil
}

func (l *Link) Remove() error {
	if l.Type() == TypeBond {
		return NewBond(netlink.NewLinkBond(*l.Attrs()), nil).remove()
	}

	return netlink.LinkDel(l)
}

// for the convenience of unit test
func getManuallyConfiguredVlans(cnName string, links []netlink.Link) []uint16 {
	prefix := utils.GetClusterNetworkDevicePrefix(cnName)
	vids := []uint16{}
	for _, link := range links {
		if !utils.HasClusterNetworkDevicePrefix(link.Attrs().Name, prefix) {
			continue
		}
		result := strings.Split(link.Attrs().Name, ".")
		if len(result) != 2 {
			// not the expected format
			continue
		}
		vid, err := strconv.Atoi(result[1])
		if err != nil {
			// not a vlan sub interface
			continue
		}
		// work on [2..4094]
		if vid > utils.DefaultVlanID && vid <= utils.MaxVlanID {
			vids = append(vids, uint16(vid)) // #nosec G115 -- vid within the expected range is safe to convert to uint16
		}
	}
	return vids
}

// user might configure sub vlan interface on a bridge directly, should always keep them
func GetManuallyConfiguredVlans(cnName string) ([]uint16, error) {
	// similar to `ip link`
	links, err := netlinksafe.LinkList()
	if err != nil {
		return nil, err
	}

	return getManuallyConfiguredVlans(cnName, links), nil
}

// mgmt must have at least one valid v4 or v6 addr no matter which family
func addrExists(link netlink.Link) (bool, error) {
	addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return false, err
	}

	for _, addr := range addrs {
		ip := addr.IP
		if ip == nil || ip.IsLoopback() {
			continue
		}

		// Check IPv4 addresses (exclude link-local APIPA 169.254.x.x)
		if ip.To4() != nil {
			if !ip.IsLinkLocalUnicast() {
				return true, nil
			}
			continue
		}

		// Check IPv6 addresses (exclude link-local fe80::/10 and multicast)
		if !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast() {
			return true, nil
		}
	}

	return false, nil
}

// GetMgmtInterface detects the management interface using a two-tier strategy:
// 1. Tier 1: If pivotIP is provided, it searches for an exact match using givenAddrExists.
// 2. Tier 2: If pivotIP is empty, it falls back to first-hit validation via addrExists.
func GetMgmtInterface(pivotIP string, family int) (string, error) {
	candidates, err := GetFilteredAndSortedMgmtLinks()
	if err != nil {
		return "", err
	}

	for _, link := range candidates {
		name := link.Attrs().Name
		var matched bool
		if pivotIP != "" {
			matched, err = givenAddrExists(link, pivotIP, family)
		} else {
			matched, err = addrExists(link)
		}

		if err != nil {
			return "", fmt.Errorf("failed to check address for interface %s: %w", name, err)
		}
		if matched {
			return name, nil
		}
	}

	if pivotIP != "" {
		return "", fmt.Errorf("no management interface found matching pivot IP %s", pivotIP)
	}
	return "", fmt.Errorf("no management interface found with a valid IP address")
}

// GetFilteredAndSortedMgmtLinks retrieves all netlink interfaces, filters them
// to keep only management bridge and VLAN subinterface prefixes, and sorts
// them deterministically by interface index (ifindex).
//
// Design rationale:
//  1. The Linux kernel assigns monotonically increasing interface indices (ifindex) upon
//     creation, providing a stable chronological ordering.
//  2. Since NetworkManager provisions management interfaces at the very beginning of node
//     initialization, sorting by ifindex allows us to safely rely on a deterministic
//     "first-hit" match for valid IP binding prior to persisting this state in CRDs.
func GetFilteredAndSortedMgmtLinks() ([]netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("failed to list netlink devices: %w", err)
	}

	var candidates []netlink.Link
	for _, link := range links {
		name := link.Attrs().Name
		if utils.HasMgmtClusterNetworkVlanDevicePrefix(name) || utils.HasMgmtClusterNetworkBridgePrefix(name) {
			candidates = append(candidates, link)
		}
	}

	// Sort deterministically by ifindex ascending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Attrs().Index < candidates[j].Attrs().Index
	})

	return candidates, nil
}

// givenAddrExists checks if the link contains an exact match for the specified pivot IP and family.
func givenAddrExists(link netlink.Link, pivotIP string, family int) (bool, error) {
	parsedPivot := net.ParseIP(pivotIP)
	if parsedPivot == nil {
		return false, fmt.Errorf("invalid pivot IP: %s", pivotIP)
	}

	addrs, err := netlink.AddrList(link, family)
	if err != nil {
		return false, err
	}

	for _, addr := range addrs {
		if addr.IP.Equal(parsedPivot) {
			return true, nil
		}
	}

	return false, nil
}
