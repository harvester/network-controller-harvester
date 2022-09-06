package vlan

import (
	"fmt"
	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
)

type Vlan struct {
	name   string
	bridge *iface.Bridge
	uplink *iface.Link
	vids   []uint16
}

func (v *Vlan) Type() string {
	return "vlanconfig"
}

// The bridge of a pure VLAN may have no latest information
// The NIC of a pure VLAN can be empty
func NewVlan(name string, vids []uint16) *Vlan {
	br := iface.NewBridge(iface.GenerateName(name, iface.BridgeSuffix))

	return &Vlan{
		name:   name,
		bridge: br,
		vids:   vids,
	}
}

func (v *Vlan) getUplink() (*iface.Link, error) {
	ifaces, err := iface.ListLinks(map[string]bool{iface.TypeDevice: true, iface.TypeBond: true})
	if err != nil {
		return nil, fmt.Errorf("get uplink of VLAN %s failed, error: %w", v.name, err)
	}

	var number int
	var uplink *iface.Link
	for _, i := range ifaces {
		if i.Attrs().MasterIndex == v.bridge.Index {
			uplink = i
			number++
		}
	}
	if number > 1 {
		return nil, fmt.Errorf("the number of uplinks of %s can not be over one, actual numbers: %d", v.name, number)
	}
	if number == 0 {
		return nil, fmt.Errorf("uplink of %s not found", v.name)
	}

	return uplink, nil
}

func GetVlan(name string) (*Vlan, error) {
	v := NewVlan(name, nil)
	if err := v.bridge.Fetch(); err != nil {
		return nil, err
	}

	uplink, err := v.getUplink()
	if err != nil {
		return nil, err
	}
	v.uplink = uplink

	v.vids, err = v.uplink.ListBridgeVlan()
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (v *Vlan) Setup(l *iface.Link) error {
	// ensure bridge and get NIC
	if err := v.bridge.Ensure(); err != nil {
		return fmt.Errorf("ensure bridge %s failed, error: %w", v.bridge.Name, err)
	}

	// set master
	if err := l.SetMaster(v.bridge, v.vids); err != nil {
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

func (v *Vlan) AddLocalArea(vid uint16, cidr string) error {
	if v.uplink == nil {
		return fmt.Errorf("bridge %s hasn't attached an uplink", v.bridge.Name)
	}
	if ok, _ := v.findVid(vid); ok {
		return nil
	}

	if err := v.uplink.AddBridgeVlan(vid); err != nil {
		return fmt.Errorf("add bridge vlanconfig %d failed, error: %w", vid, err)
	}
	v.vids = append(v.vids, vid)

	if cidr == "" {
		return nil
	}

	if err := iface.EnsureRouteViaGateway(cidr); err != nil {
		return fmt.Errorf("ensure %s to route via gateway failed, error: %w", cidr, err)
	}

	return nil
}

func (v *Vlan) RemoveLocalArea(vid uint16, cidr string) error {
	if v.uplink == nil {
		return fmt.Errorf("bridge %s hasn't attached an uplink", v.bridge.Name)
	}

	ok, index := v.findVid(vid)
	if !ok {
		return nil
	}

	if err := v.uplink.DelBridgeVlan(vid); err != nil {
		return fmt.Errorf("remove bridge vlanconfig %d failed, error: %w", vid, err)
	}
	v.vids = append(v.vids[:index], v.vids[index+1:]...)

	if cidr == "" {
		return nil
	}

	if err := iface.DeleteRouteViaGateway(cidr); err != nil {
		return fmt.Errorf("delete route with dst %s via gateway failed, error: %w", cidr, err)
	}

	return nil
}

func (v *Vlan) ListLocalArea() []uint16 {
	return v.vids
}

func (v *Vlan) Bridge() *iface.Bridge {
	return v.bridge
}

func (v *Vlan) Uplink() *iface.Link {
	return v.uplink
}

func (v *Vlan) findVid(vid uint16) (bool, int) {
	for i, v := range v.vids {
		if v == vid {
			return true, i
		}
	}
	return false, -1
}
