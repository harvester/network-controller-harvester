package vlan

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type Vlan struct {
	name   string
	bridge *iface.Bridge
	uplink *iface.Link
}

func (v *Vlan) Type() string {
	return "vlanconfig"
}

// The bridge of a pure VLAN may have no latest information
// The NIC of a pure VLAN can be empty
func NewVlan(name string) *Vlan {
	br := iface.NewBridge(utils.GenerateBridgeName(name))

	return &Vlan{
		name:   name,
		bridge: br,
	}
}

func (v *Vlan) getUplink() (*iface.Link, error) {
	l, err := netlink.LinkByName(utils.GenerateBondName(v.name))
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

func (v *Vlan) Teardown() error {
	logrus.Info("start to tear down VLAN network")
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

	logrus.Info("tear down VLAN network successfully")
	return nil
}

func (v *Vlan) AddLocalAreas(vis *utils.VlanIDSet) error {
	if vis == nil {
		return nil
	}
	if v.uplink == nil {
		return fmt.Errorf("bridge %s hasn't attached with an uplink", v.bridge.Name)
	}
	if err := vis.WalkVIDs("add bridge vlanconfig", v.uplink.AddBridgeVlan); err != nil {
		return nil
	}
	return nil
}

func (v *Vlan) RemoveLocalAreas(vis *utils.VlanIDSet) error {
	if vis == nil {
		return nil
	}
	if v.uplink == nil {
		return fmt.Errorf("bridge %s hasn't attached with an uplink", v.bridge.Name)
	}

	if err := vis.WalkVIDs("remove bridge vlanconfig", v.uplink.DelBridgeVlan); err != nil {
		return nil
	}
	return nil
}

func (v *Vlan) ToVlanIDSet() (*utils.VlanIDSet, error) {
	// the NewVlan returned vlan never has an empty uplink, skip check it
	return v.uplink.ToVlanIDSet()
}

func (v *Vlan) Bridge() *iface.Bridge {
	return v.bridge
}

func (v *Vlan) Uplink() *iface.Link {
	return v.uplink
}
