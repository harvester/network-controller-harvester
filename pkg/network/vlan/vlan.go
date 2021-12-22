package vlan

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/network"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/network/monitor"
)

type Vlan struct {
	bridge      *iface.Bridge
	nic         *iface.Link
	status      *network.Status
	eventSender network.EventSender
	mgmtNetwork network.Network
	vids        []uint16
}

const (
	BridgeName = "harvester-br0"
)

func (v *Vlan) Type() string {
	return "vlan"
}

// The bridge of a pure VLAN may have no latest information
// The NIC of a pure VLAN can be empty
func NewVlan(eventSender network.EventSender, mgmtNetwork network.Network, vids []uint16) *Vlan {
	br := iface.NewBridge(BridgeName)
	return &Vlan{
		bridge:      br,
		status:      &network.Status{},
		eventSender: eventSender,
		mgmtNetwork: mgmtNetwork,
		vids:        vids,
	}
}

func (v *Vlan) getSlaveNIC() (*iface.Link, error) {
	nics, err := iface.ListLinks(map[string]bool{iface.TypeDevice: true, iface.TypeBond: true})
	if err != nil {
		return nil, fmt.Errorf("get NICs failed, error: %w", err)
	}

	var number int
	var slaveNIC string
	for _, n := range nics {
		if n.LinkAttrs().MasterIndex == v.bridge.Index() {
			slaveNIC = n.Name()
			number++
		}
	}
	if number > 1 {
		return nil, fmt.Errorf("the number of slave NICs can not be over one, actual numbers: %d", number)
	}

	if number == 0 {
		return nil, SlaveNotFoundError{fmt.Errorf("slave of %s not found", v.bridge.Name())}
	}

	return iface.GetLinkByName(slaveNIC)
}

func GetVlan(mgmtNetwork network.Network) (*Vlan, error) {
	v := NewVlan(nil, mgmtNetwork, nil)
	if err := v.bridge.Fetch(); err != nil {
		return nil, err
	}

	nic, err := v.getSlaveNIC()
	if err != nil {
		return nil, err
	}
	v.nic = nic

	return v, nil
}

func (v *Vlan) Setup(nic string) error {
	klog.Info("start to setup VLAN network")
	// ensure bridge and get NIC
	if err := v.bridge.Ensure(); err != nil {
		return fmt.Errorf("ensure bridge %s failed, error: %w", v.bridge.Name(), err)
	}
	l, err := iface.GetLinkByName(nic)
	if err != nil {
		return fmt.Errorf("get NIC %s failed, error: %w", nic, err)
	}
	if l.LinkAttrs().OperState == netlink.OperDown {
		return fmt.Errorf("NIC %s is down", l.Name())
	}

	// Use the same NIC with the flannel network
	if l.Index() == v.mgmtNetwork.NIC().Index() {
		if err := v.bridge.SyncIPv4Addr(l); err != nil {
			return err
		}
		if err := v.bridge.ToLink().AddRoutes(l); err != nil {
			return err
		}
	}

	// set master
	if err := l.SetMaster(v.bridge, v.vids); err != nil {
		return err
	}
	// delete routes of NIC
	if err := l.DeleteRoutes(); err != nil {
		return err
	}

	v.nic = l
	v.startMonitor()

	klog.Info("setup VLAN network successfully")
	return nil
}

// Note: It's required to call function GetVlanWithNic before tearing down VLAN.
func (v *Vlan) Teardown() error {
	klog.Info("start to tear down VLAN network")
	if v.nic == nil {
		return fmt.Errorf("vlan network hasn't attached a NIC")
	}

	v.stopMonitor()

	// delete iptables rule
	if err := v.bridge.ToLink().DeleteIptForward(); err != nil {
		return err
	}
	// set no master, VIDs will be auto-removed
	if err := v.nic.SetNoMaster(); err != nil {
		return fmt.Errorf("set %s no master failed, error: %w", v.nic.Name(), err)
	}
	// append route for NIC
	if err := v.nic.AddRoutes(v.bridge); err != nil {
		return err
	}
	// delete route of bridge explicitly
	if err := v.bridge.ToLink().DeleteRoutes(); err != nil {
		return err
	}
	// delete IPv4 address of bridge
	if err := v.bridge.ClearAddr(); err != nil {
		return err
	}

	klog.Info("tear down VLAN network successfully")
	return nil
}

func (v *Vlan) startMonitor() {
	bridgeMonitorHandler := monitor.Handler{
		NewLink: v.afterLinkDown,
	}

	nicMonitorHandler := monitor.Handler{
		NewLink: v.afterLinkDown,
	}

	w := network.GetWatcher()

	w.EmptyLink()
	w.AddLink(v.bridge.Index(), bridgeMonitorHandler)
	w.AddLink(v.nic.Index(), nicMonitorHandler)
}

func (v *Vlan) stopMonitor() {
	network.GetWatcher().DelLink(v.bridge.Index())
	network.GetWatcher().DelLink(v.nic.Index())
}

func (v *Vlan) AddLocalArea(id int, cidr string) error {
	if v.nic == nil {
		return fmt.Errorf("physical nic vlan network")
	}

	if err := v.nic.AddBridgeVlan(uint16(id)); err != nil {
		return fmt.Errorf("add bridge vlan %d failed, error: %w", id, err)
	}

	if cidr == "" {
		return nil
	}

	if err := iface.EnsureRouteViaGateway(cidr); err != nil {
		return fmt.Errorf("ensure %s to route via gateway failed, error: %w", cidr, err)
	}
	// update routes of bridge
	if v.NIC().Index() == v.mgmtNetwork.NIC().Index() {
		return v.bridge.Fetch()
	}

	return nil
}

func (v *Vlan) RemoveLocalArea(id int, cidr string) error {
	if v.nic == nil {
		return fmt.Errorf("physical nic vlan network")
	}

	if err := v.nic.DelBridgeVlan(uint16(id)); err != nil {
		return fmt.Errorf("remove bridge vlan %d failed, error: %w", id, err)
	}

	if cidr == "" {
		return nil
	}

	if err := iface.DeleteRouteViaGateway(cidr); err != nil {
		return fmt.Errorf("delete route with dst %s via gateway failed, error: %w", cidr, err)
	}
	if v.NIC().Index() == v.mgmtNetwork.NIC().Index() {
		return v.bridge.Fetch()
	}

	return nil
}

func (v *Vlan) Status(condition network.Condition) (*network.Status, error) {
	if err := v.bridge.Fetch(); err != nil {
		return nil, err
	}
	if v.nic != nil {
		if err := v.nic.Fetch(); err != nil {
			return nil, err
		}
	}

	return &network.Status{
		Condition: condition,
		IFaces: map[string]iface.IFace{
			v.bridge.Name(): v.bridge,
			v.nic.Name():    v.nic,
		},
	}, nil
}

func (v *Vlan) afterLinkDown(update netlink.LinkUpdate) {
	if update.Link.Attrs().OperState == netlink.OperDown && (update.Link.Attrs().Flags&net.FlagUp) == 0 && update.Change == 1 {
		event := &network.Event{
			EventType: v1.EventTypeWarning,
			Reason:    "LinkDown",
			Message:   fmt.Sprintf("Link %s has been down atypically and the controller will try to set it up", update.Link.Attrs().Name),
		}
		if v.eventSender != nil {
			if err := v.eventSender.SendEvent(event, v.Type()); err != nil {
				klog.Errorf("recorde event failed, error: %s", err.Error())
			}
		}

		if err := netlink.LinkSetUp(update.Link); err != nil {
			klog.Errorf("link %s set up failed, error: %s", update.Link.Attrs().Name, err.Error())
			return
		}

		// recover routes
		if update.Link.Attrs().Index == v.bridge.Index() {
			if err := v.bridge.ToLink().AddRoutes(v.bridge); err != nil {
				klog.Error(err)
				return
			}
		}
		if update.Link.Attrs().Index == v.nic.Index() {
			if err := v.nic.DeleteRoutes(); err != nil {
				klog.Error(err)
			}
		}
	}
}

func (v *Vlan) NIC() iface.IFace {
	return v.nic
}
