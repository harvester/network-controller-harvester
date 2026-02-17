package iface

import (
	"fmt"
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

// DelBridgeVlan adds a new vlan filter entry
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
