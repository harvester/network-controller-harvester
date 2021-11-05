package network

import (
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
)

// network type
type Network interface {
	Type() string
	Setup(nic string) error
	Teardown() error
	NIC() iface.IFace
}

type IsolatedNetwork interface {
	Network
	AddLocalArea(id int, cidr string) error
	RemoveLocalArea(id int, cidr string) error
}

type Status struct {
	Condition Condition
	IFaces    map[string]iface.IFace
}

type Condition struct {
	Normal  bool
	Message string
}
