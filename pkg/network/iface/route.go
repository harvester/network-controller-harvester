package iface

import (
	"encoding/json"
	"errors"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"
)

const defaultIPv4Route = "0.0.0.0/0"

func Route2String(route netlink.Route) (string, error) {
	bytes, err := json.Marshal(route)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func String2Route(route string) (*netlink.Route, error) {
	r := &netlink.Route{}

	if err := json.Unmarshal([]byte(route), r); err != nil {
		return nil, err
	}

	return r, nil
}

func getGateway() (gateway net.IP, linkIndex int, err error) {
	var routes []netlink.Route
	routes, err = netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return
	}

	for _, route := range routes {
		//Find the default route with gateway
		if route.Gw != nil && route.Dst == nil || route.Dst.String() == defaultIPv4Route {
			linkIndex = route.LinkIndex
			gateway = route.Gw
			break
		}
	}

	return
}

// EnsureRouteViaGateway will add the route via gateway if not existing
func EnsureRouteViaGateway(cidr string) error {
	if cidr == "" {
		return nil
	}

	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	gateway, linkIndex, err := getGateway()
	if err != nil {
		return err
	}

	routes, err := netlink.RouteGet(ip)
	if err != nil {
		return err
	}

	for _, route := range routes {
		if route.Gw != nil {
			return nil
		}
	}

	route := &netlink.Route{
		LinkIndex: linkIndex,
		Dst:       network,
		Gw:        gateway,
		Protocol:  netlink.FAMILY_V4,
	}
	if err := netlink.RouteAdd(route); err != nil && err != syscall.EEXIST {
		return err
	} else if err == nil {
		klog.Infof("add route: %+v", route)
	}

	return nil
}

func DeleteRouteViaGateway(cidr string) error {
	if cidr == "" {
		return nil
	}

	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	gateway, linkIndex, err := getGateway()
	if err != nil {
		return err
	}

	route := &netlink.Route{
		Dst:       network,
		Gw:        gateway,
		LinkIndex: linkIndex,
	}
	if err := netlink.RouteDel(route); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	} else if err == nil {
		klog.Infof("delete route: %+v", route)
	}

	return nil
}
