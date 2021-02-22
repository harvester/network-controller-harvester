package iface

import "github.com/vishvananda/netlink"

type IFace interface {
	Index() int
	Name() string
	Type() string
	LinkAttrs() *netlink.LinkAttrs
	Addr() []netlink.Addr
	Routes() []netlink.Route
}
