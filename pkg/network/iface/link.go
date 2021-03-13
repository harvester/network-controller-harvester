package iface

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"syscall"
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

func (l *Link) SetMaster(br *Bridge, vids []uint16) error {
	if err := l.setRules4DHCP(); err != nil {
		return err
	}
	if l.link.Attrs().MasterIndex == br.bridge.Index {
		return nil
	}
	if err := netlink.LinkSetMaster(l.link, br.bridge); err != nil {
		return fmt.Errorf("%s set %s as master failed, error: %w", l.Name(), br.Name(), err)
	}
	for _, vid := range vids {
		if err := l.AddBridgeVlan(vid); err != nil {
			return err
		}
	}

	return nil
}

func (l *Link) SetNoMaster() error {
	if err := l.unsetRules4DHCP(); err != nil {
		return err
	}

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
func (l *Link) setRules4DHCP() error {
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

func (l *Link) unsetRules4DHCP() error {
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

func (l *Link) AddRoutes(link IFace) error {
	// Rearrange routes to let routes with gateway be configured later than the ones without.
	// Otherwise, error syscall.ENETUNREACH, exactly "network is unreachable", may occur.
	routes := link.Routes()
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Gw == nil
	})
	for _, route := range routes {
		route.LinkIndex = l.Index()
		err := netlink.RouteAppend(&route)
		if err != nil && !errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("append route failed, route: %+v, error: %w", route, err)
		} else if err == nil {
			klog.Infof("append route: %+v", route)
		} else {
			klog.Infof("ignore existing route: %+v", route)
		}
	}

	return nil
}

func (l *Link) DeleteRoutes() error {
	for _, route := range l.routes {
		if err := netlink.RouteDel(&route); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("delete route failed, route: %+v, error: %w", route, err)
		}
		klog.Infof("delete route %+v", route)
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
