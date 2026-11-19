package vlan

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type Vlan struct {
	name   string
	bridge *iface.Bridge
	// uplinks are the bridge ports that connect to the physical network: the single bond <cn>-bo in bond mode,
	// or one entry per NIC in isolated mode. uplinks[0] is kept as the primary uplink for callers of Uplink().
	uplinks []*iface.Link
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

// getUplinks returns the bond <cn>-bo if it exists (bond mode), otherwise the physical NICs attached directly to the
// bridge (isolated mode). It returns a LinkNotFoundError if the bridge has no uplink.
func (v *Vlan) getUplinks() ([]*iface.Link, error) {
	l, err := netlink.LinkByName(utils.GenerateBondName(v.name))
	if err == nil {
		return []*iface.Link{iface.NewLink(l)}, nil
	}
	if !errors.As(err, &netlink.LinkNotFoundError{}) {
		return nil, err
	}

	nics, err := iface.ListBridgePorts(v.bridge.Index, iface.TypeDevice)
	if err != nil {
		return nil, err
	}
	if len(nics) == 0 {
		return nil, netlink.LinkNotFoundError{}
	}

	return nics, nil
}

func (v *Vlan) GetBridgelink() (*iface.Link, error) {
	l, err := netlink.LinkByName(utils.GenerateBridgeName(v.name))
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

	uplinks, err := v.getUplinks()
	if err != nil {
		return nil, err
	}
	v.uplinks = uplinks

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
	v.uplinks = []*iface.Link{l}

	return nil
}

// SetupIsolated attaches every NIC directly to the bridge as an isolated bridge port (no bond). NICs that are
// attached to the bridge but no longer listed are detached. Each NIC is isolated before it is brought up, so the
// bridge never forwards between two uplinks, not even briefly while the ports are being added.
// RemoveBridge deletes the bridge of the cluster network if it exists. It is used to tear down a bridge that has no
// uplink left, which GetVlan reports as not found.
func RemoveBridge(name string) error {
	br := iface.NewBridge(utils.GenerateBridgeName(name))
	if err := br.Fetch(); err != nil {
		return nil
	}
	return iface.NewLink(br).Remove()
}

func (v *Vlan) SetupIsolated(nics []string, attrs *iface.UplinkAttrs) error {
	// check every NIC before creating the bridge, so a wrong NIC name doesn't leave an empty bridge behind
	brIndex := 0
	if err := v.bridge.Fetch(); err == nil {
		brIndex = v.bridge.Index
	}
	for _, n := range nics {
		l, err := netlink.LinkByName(n)
		if err != nil {
			return fmt.Errorf("get link %s failed, error: %w", n, err)
		}
		if master := l.Attrs().MasterIndex; master != 0 && master != brIndex {
			return fmt.Errorf("%s has been enslaved by the link with index %d", n, master)
		}
	}

	if err := v.bridge.Ensure(); err != nil {
		return fmt.Errorf("ensure bridge %s failed, error: %w", v.bridge.Name, err)
	}

	wanted := make(map[string]bool, len(nics))
	for _, n := range nics {
		wanted[n] = true
	}
	attached, err := iface.ListBridgePorts(v.bridge.Index, iface.TypeDevice)
	if err != nil {
		return err
	}
	for _, l := range attached {
		if !wanted[l.Attrs().Name] {
			if err := l.SetNoMaster(); err != nil {
				return fmt.Errorf("detach %s from %s failed, error: %w", l.Attrs().Name, v.bridge.Name, err)
			}
		}
	}

	uplinks := make([]*iface.Link, 0, len(nics))
	for _, n := range nics {
		l, err := iface.EnsureIsolatedPort(n, v.bridge, attrs)
		if err != nil {
			return err
		}
		uplinks = append(uplinks, l)
	}
	v.uplinks = uplinks

	return nil
}

func (v *Vlan) Teardown() error {
	logrus.Info("start to tear down VLAN network")
	if len(v.uplinks) == 0 {
		return fmt.Errorf("bridge %s hasn't attached an uplink", v.bridge.Name)
	}

	for _, uplink := range v.uplinks {
		// set no master, VIDs will be auto-removed
		if err := uplink.SetNoMaster(); err != nil {
			return fmt.Errorf("set %s no master failed, error: %w", uplink.Attrs().Name, err)
		}
		// the bond is owned by the controller and deleted; NICs of isolated mode are only detached
		if uplink.Type() != iface.TypeBond {
			continue
		}
		if err := uplink.Remove(); err != nil {
			return fmt.Errorf("delete uplink %s failed, error: %w", uplink.Attrs().Name, err)
		}
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
	if len(v.uplinks) == 0 {
		return fmt.Errorf("bridge %s hasn't attached with an uplink", v.bridge.Name)
	}
	for _, uplink := range v.uplinks {
		if err := vis.WalkVIDs("add bridge vlanconfig", uplink.AddBridgeVlan); err != nil {
			return err
		}
	}
	return nil
}

func (v *Vlan) RemoveLocalAreas(vis *utils.VlanIDSet) error {
	if vis == nil {
		return nil
	}
	if len(v.uplinks) == 0 {
		return fmt.Errorf("bridge %s hasn't attached with an uplink", v.bridge.Name)
	}

	for _, uplink := range v.uplinks {
		if err := vis.WalkVIDs("remove bridge vlanconfig", uplink.DelBridgeVlan); err != nil {
			return err
		}
	}
	return nil
}

// SyncLocalAreas makes the VIDs of every uplink match target: each uplink is diffed against target on its own, so a
// NIC that misses a VID (e.g. just attached in isolated mode) or keeps a stale one is corrected independently.
func (v *Vlan) SyncLocalAreas(target *utils.VlanIDSet) (added, removed uint32, err error) {
	if len(v.uplinks) == 0 {
		return 0, 0, fmt.Errorf("bridge %s hasn't attached with an uplink", v.bridge.Name)
	}

	for _, uplink := range v.uplinks {
		existing, err := uplink.ToVlanIDSet()
		if err != nil {
			return added, removed, err
		}
		if existing == nil {
			existing = utils.NewVlanIDSet()
		}
		toAdd, toRemove, err := target.Diff(existing)
		if err != nil {
			return added, removed, err
		}
		if err := toAdd.WalkVIDs("add bridge vlanconfig", uplink.AddBridgeVlan); err != nil {
			return added, removed, err
		}
		if err := toRemove.WalkVIDs("remove bridge vlanconfig", uplink.DelBridgeVlan); err != nil {
			return added, removed, err
		}
		added += toAdd.GetVlanCount()
		removed += toRemove.GetVlanCount()
	}

	return added, removed, nil
}

// ToVlanIDSet returns the VIDs of the primary uplink
func (v *Vlan) ToVlanIDSet() (*utils.VlanIDSet, error) {
	// the GetVlan returned vlan never has an empty uplink list, skip check it
	return v.uplinks[0].ToVlanIDSet()
}

func (v *Vlan) Bridge() *iface.Bridge {
	return v.bridge
}

func (v *Vlan) Uplink() *iface.Link {
	if len(v.uplinks) == 0 {
		return nil
	}
	return v.uplinks[0]
}

func (v *Vlan) Uplinks() []*iface.Link {
	return v.uplinks
}
