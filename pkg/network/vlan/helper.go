package vlan

import (
	"github.com/rancher/harvester-network-controller/pkg/network/iface"
)

type Helper struct {
	EventSender func(event *Event) error
	Resetter    func() error
}

type Event struct {
	EventType string
	Reason    string
	Message   string
}

type Status struct {
	Condition Condition
	IFaces    map[string]iface.IFace
}

type Condition struct {
	Normal  bool
	Message string
}
