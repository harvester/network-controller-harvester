package iface

import (
	"context"
	"fmt"
	"time"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"
	"k8s.io/utils/exec"
	"k8s.io/utils/net/ebtables"
)

const defaultPVID = uint16(1)

type Link struct {
	link   netlink.Link
	addr   []netlink.Addr
	routes []netlink.Route
}

func GetLink(name string) (*Link, error) {
	if name == "" {
		return nil, fmt.Errorf("link name could not be empty string")
	}

	l, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("could not lookup link, error: %w, link: %s", err, name)
	}
	addr, err := netlink.AddrList(l, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("list IPv4 address of %s failed, error: %w", l.Attrs().Name, err)
	}
	routes, err := netlink.RouteList(l, netlink.FAMILY_V4)
	if err != nil {
		return nil, fmt.Errorf("list routes of %s failed, error: %w", l.Attrs().Name, err)
	}

	return &Link{
		link:   l,
		addr:   addr,
		routes: routes,
	}, nil
}

// AddBridgeVlan adds a new vlan filter entry
// Equivalent to: `bridge vlan add dev DEV vid VID master`
func (l *Link) AddBridgeVlan(vid uint16) error {
	if vid == defaultPVID {
		return nil
	}

	if err := netlink.BridgeVlanAdd(l.link, vid, false, false, false, true); err != nil {
		return fmt.Errorf("add iface vlan failed, error: %v, link: %s, vid: %d", err, l.Name(), vid)
	}

	return nil
}

// DelBridgeVlan adds a new vlan filter entry
// Equivalent to: `bridge vlan del dev DEV vid VID master`
func (l *Link) DelBridgeVlan(vid uint16) error {
	if vid == defaultPVID {
		return nil
	}

	if err := netlink.BridgeVlanDel(l.link, vid, false, false, false, true); err != nil {
		return fmt.Errorf("delete iface vlan failed, error: %v, link: %s, vid: %d", err, l.Name(), vid)
	}

	return nil
}

func (l *Link) SetMaster(br *Bridge) error {
	if l.link.Attrs().MasterIndex == br.bridge.Index {
		return nil
	}

	if l.link.Attrs().OperState == netlink.OperDown {
		if err := netlink.LinkSetUp(l.link); err != nil {
			return err
		}
	}

	klog.Infof("%s set master as %s", l.Name(), br.Name())
	return netlink.LinkSetMaster(l.link, br.bridge)
}

func (l *Link) SetNoMaster() error {
	if l.LinkAttrs().MasterIndex == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	var isUp bool
	if l.LinkAttrs().OperState == netlink.OperUp {
		isUp = true
		go l.reportDown(ctx, cancel)
	}

	klog.Infof("%s set no master", l.Name())
	if err := netlink.LinkSetNoMaster(l.link); err != nil {
		return err
	}

	if isUp {
		select {
		case <-ctx.Done():
			return netlink.LinkSetUp(l.link)
		case <-time.After(time.Minute):
			klog.Infof("Waiting for link down event timeout")
		}
	}

	return nil
}

func (l *Link) reportDown(ctx context.Context, cancel context.CancelFunc) {
	linkCh := make(chan netlink.LinkUpdate)

	if err := netlink.LinkSubscribe(linkCh, ctx.Done()); err != nil {
		klog.Errorf("subscribe link failed, error: %s", err.Error())
		return
	}

	for update := range linkCh {
		if int(update.Index) == l.Index() && update.Link.Attrs().OperState == netlink.OperDown {
			klog.Infof("%+v/n%+v", update, update.Link)
			cancel()
			return
		}
	}
}

// allow to receive DHCP packages after attaching with bridge
func (l *Link) SetRules4DHCP() error {
	executor := exec.New()
	runner := ebtables.New(executor)
	var ruleArgs []string

	ruleArgs = append(ruleArgs, "-p", "IPv4", "-d", l.link.Attrs().HardwareAddr.String(), "-i", l.Name(),
		"--ip-proto", "udp", "--ip-dport", "68", "-j", "DROP")
	_, err := runner.EnsureRule(ebtables.Append, ebtables.TableBroute, ebtables.ChainBrouting, ruleArgs...)
	if err != nil {
		return fmt.Errorf("set ebtables rules failed, error: %w", err)
	}

	return nil
}

func (l *Link) UnsetRules4DHCP() error {
	executor := exec.New()
	runner := ebtables.New(executor)
	var ruleArgs []string

	ruleArgs = append(ruleArgs, "-p", "IPv4", "-d", l.link.Attrs().HardwareAddr.String(), "-i", l.Name(),
		"--ip-proto", "udp", "--ip-dport", "68", "-j", "DROP")
	if err := runner.DeleteRule(ebtables.TableBroute, ebtables.ChainBrouting, ruleArgs...); err != nil {
		return fmt.Errorf("delete ebtables rules failed, error: %w", err)
	}

	return nil
}

func (l *Link) ReplaceRoutes(replaced *Link) error {
	routeList, err := netlink.RouteList(replaced.link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("could not list routes, error: %w, link: %s", err, replaced.Name())
	}

	for _, route := range routeList {
		route.LinkIndex = l.Index()
		if err := netlink.RouteReplace(&route); err != nil {
			klog.Warningf("could not replace route %v and it will be removed", route)
			route.LinkIndex = replaced.Index()
			// remove route rule that can not be replaced, such as
			// the auto-generated route `172.16.0.0/16 dev harvester-br0 proto kernel scope link src 172.16.0.76`
			if err := netlink.RouteDel(&route); err != nil {
				klog.Errorf("could not delete route, error: %s, route: %v", err.Error(), route)
			}
		} else {
			klog.Infof("replace route, route: %+v", route)
		}
	}

	return nil
}

func (l *Link) Index() int {
	return l.link.Attrs().Index
}

func (l *Link) Name() string {
	return l.link.Attrs().Name
}

func (l *Link) Type() string {
	return l.link.Type()
}

func (l *Link) LinkAttrs() *netlink.LinkAttrs {
	return l.link.Attrs()
}

func (l *Link) Addr() []netlink.Addr {
	return l.addr
}

func (l *Link) Routes() []netlink.Route {
	return l.routes
}

func (l *Link) Fetch() error {
	link, err := netlink.LinkByName(l.Name())
	if err != nil {
		return fmt.Errorf("refresh link %s failed, error: %w", l.Name(), err)
	}
	addr, err := netlink.AddrList(l.link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("refresh addresses of link %s failed, error: %w", l.Name(), err)
	}
	routes, err := netlink.RouteList(l.link, netlink.FAMILY_V4)
	if err != nil {
		return fmt.Errorf("refresh routes of link %s failed, error: %w", l.Name(), err)
	}

	l.link = link
	l.addr = addr
	l.routes = routes

	return nil
}
