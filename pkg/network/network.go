package network

import (
	"github.com/vishvananda/netlink"

	"github.com/rancher/harvester-network-controller/pkg/network/iface"
)

// network type
type Network interface {
	Type() string
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

type Status struct {
	Condition Condition
	IFaces    map[string]iface.IFace
}

type Condition struct {
	Normal  bool
	Message string
}
