package vlan

import (
	"fmt"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
)

type Vlan struct {
	name   string
	bridge *iface.Bridge
	uplink *iface.Link
}

type LocalArea struct {
	Vid  uint16
	Cidr string
}

func (v *Vlan) Type() string {
	return "vlanconfig"
}

// The bridge of a pure VLAN may have no latest information
// The NIC of a pure VLAN can be empty
func NewVlan(name string) *Vlan {
	br := iface.NewBridge(iface.GenerateName(name, iface.BridgeSuffix))

	return &Vlan{
		name:   name,
		bridge: br,
	}
}

func (v *Vlan) getUplink() (*iface.Link, error) {
	l, err := netlink.LinkByName(iface.GenerateName(v.name, iface.BondSuffix))
	if err != nil {
		return nil, err
	}

	return iface.NewLink(l), nil
}

func GetVlan(name string) (*Vlan, error) {
	v := NewVlan(name)
	if err := v.bridge.Fetch(); err != nil {
		return nil, err
	}

	uplink, err := v.getUplink()
	if err != nil {
		return nil, err
	}
	v.uplink = uplink

	return v, nil
}

func (v *Vlan) Setup(l *iface.Link) error {
	// ensure bridge and get NIC
	if err := v.bridge.Ensure(); err != nil {
		return fmt.Errorf("ensure bridge %s failed, error: %w", v.bridge.Name, err)
	}

	// set master
	if err := l.SetMaster(v.bridge); err != nil {
		return err
	}
	v.uplink = l

	return nil
}

// Note: It's required to call function GetVlanWithNic before tearing down VLAN.
func (v *Vlan) Teardown() error {
	klog.Info("start to tear down VLAN network")
	if v.uplink == nil {
		return fmt.Errorf("bridge %s hasn't attached an uplink", v.bridge.Name)
	}

	// set no master, VIDs will be auto-removed
	if err := v.uplink.SetNoMaster(); err != nil {
		return fmt.Errorf("set %s no master failed, error: %w", v.uplink.Attrs().Name, err)
	}

	if err := v.uplink.Remove(); err != nil {
		return fmt.Errorf("delete uplink %s failed, error: %w", v.uplink.Attrs().Name, err)
	}

	if err := iface.NewLink(v.bridge).Remove(); err != nil {
		return fmt.Errorf("delete bridge %s failed, error: %w", v.bridge.Name, err)
	}

	klog.Info("tear down VLAN network successfully")
	return nil
}

func (v *Vlan) AddLocalArea(la *LocalArea) error {
	if v.uplink == nil {
		return fmt.Errorf("bridge %s hasn't attached an uplink", v.bridge.Name)
	}

	if err := v.uplink.AddBridgeVlan(la.Vid); err != nil {
		return fmt.Errorf("add bridge vlanconfig %d failed, error: %w", la.Vid, err)
	}

	return nil
}

func (v *Vlan) RemoveLocalArea(la *LocalArea) error {
	if v.uplink == nil {
		return fmt.Errorf("bridge %s hasn't attached an uplink", v.bridge.Name)
	}

	if err := v.uplink.DelBridgeVlan(la.Vid); err != nil {
		return fmt.Errorf("remove bridge vlanconfig %d failed, error: %w", la.Vid, err)
	}

	return nil
}

func (v *Vlan) Bridge() *iface.Bridge {
	return v.bridge
}

func (v *Vlan) Uplink() *iface.Link {
	return v.uplink
}
