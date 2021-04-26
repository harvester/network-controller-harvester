package network

import (
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
)

// network type
type Network interface {
	Type() string
	Setup(nic string, vids []uint16) error
	Teardown() error
}

type IsolatedNetwork interface {
	Network
	AddLocalArea(id int) error
	RemoveLocalArea(id int) error
}

type Status struct {
	Condition Condition
	IFaces    map[string]iface.IFace
}

type Condition struct {
	Normal  bool
	Message string
}
