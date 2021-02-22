package network

import (
	"github.com/vishvananda/netlink"
)

// network type
type Network interface {
	Setup(nic string, conf Config) error
	Repeal() error
}

type IsolatedNetwork interface {
	Network
	AddLocalArea(id int) error
	RemoveLocalArea(id int) error
}

type Config struct {
	Routes []*netlink.Route
}
