package vlan

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"github.com/rancher/harvester-network-controller/pkg/network"
	"github.com/rancher/harvester-network-controller/pkg/network/iface"
	"github.com/rancher/harvester-network-controller/pkg/network/monitor"
)

type Vlan struct {
	bridge *iface.Bridge
	nic    *iface.Link
	status *Status
	helper *Helper
}

const BridgeName = "harvester-br0"

// The bridge of a pure VLAN may have no latest information
// The NIC of a pure VLAN can be empty
func NewPureVlan(helper *Helper) *Vlan {
	br := iface.NewBridge(BridgeName)
	return &Vlan{
		bridge: br,
		status: &Status{},
		helper: helper,
	}
}

// A Vlan with NIC is has been configured on node
func NewVlanWithNic(nic string, helper *Helper) (*Vlan, error) {
	v := NewPureVlan(helper)
	if err := v.bridge.Fetch(); err != nil {
		return nil, err
	}
	l, err := iface.GetLink(nic)
	if err != nil {
		return nil, err
	}
	v.nic = l

	return v, nil
}

func (v *Vlan) Setup(nic string, conf network.Config) error {
	klog.Info("start setup VLAN network")

	// ensure bridge
	if err := v.bridge.Ensure(); err != nil {
		return fmt.Errorf("ensure bridge %s failed, error: %w", v.bridge.Name(), err)
	}

	l, err := iface.GetLink(nic)
	if err != nil {
		return err
	}

	// setup L2 layer network
	if err = v.setupL2(l); err != nil {
		return err
	}
	// setup L3 layer network
	if err = v.setupL3(l, conf.Routes); err != nil {
		return err
	}

	v.nic = l
	v.startMonitor()
	return nil
}

func (v *Vlan) startMonitor() {
	bridgeMonitorHandler := monitor.Handler{
		NewLink: v.afterLinkDown,
		DelLink: v.afterDelBridge,
	}

	nicMonitorHandler := monitor.Handler{
		NewLink: v.afterLinkDown,
		NewAddr: v.afterModifyNicIP,
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

func (v *Vlan) setupL2(nic *iface.Link) error {
	return nic.SetMaster(v.bridge)
}

func (v *Vlan) unsetL2() error {
	return v.nic.SetNoMaster()
}

func (v *Vlan) setupL3(nic *iface.Link, routes []*netlink.Route) error {
	klog.Infof("set L3")
	// set ebtables rules
	if err := nic.SetRules4DHCP(); err != nil {
		return err
	}

	// configure IPv4 address
	return v.bridge.ConfigIPv4AddrFromSlave(nic, routes)
}

func (v *Vlan) unsetL3() error {
	if err := v.nic.UnsetRules4DHCP(); err != nil {
		return err
	}

	// resume nic's routes
	if err := v.nic.ReplaceRoutes(v.bridge.ToLink()); err != nil {
		return err
	}

	return nil
}

func (v *Vlan) AddLocalArea(id int) error {
	if v.nic == nil {
		return fmt.Errorf("physical nic vlan network")
	}
	return v.nic.AddBridgeVlan(uint16(id))
}

func (v *Vlan) RemoveLocalArea(id int) error {
	if v.nic == nil {
		return fmt.Errorf("physical nic vlan network")
	}
	return v.nic.DelBridgeVlan(uint16(id))
}

func (v *Vlan) Repeal() error {
	klog.Info("start repeal VLAN network")

	if v.nic == nil {
		return fmt.Errorf("vlan network has't attached a NIC")
	}

	v.stopMonitor()

	if err := v.unsetL3(); err != nil {
		return err
	}

	if err := v.unsetL2(); err != nil {
		return err
	}

	return nil
}

func (v *Vlan) Status(condition Condition) (*Status, error) {
	if err := v.bridge.Fetch(); err != nil {
		return nil, err
	}
	if v.nic != nil {
		if err := v.nic.Fetch(); err != nil {
			return nil, err
		}
	}

	return &Status{
		Condition: condition,
		IFaces: map[string]iface.IFace{
			v.bridge.Name(): v.bridge,
			v.nic.Name():    v.nic,
		},
	}, nil
}

func (v *Vlan) afterDelBridge(update netlink.LinkUpdate) {
	message := fmt.Sprintf("Bridge device %s has been deleted and the controller will try to restore", update.Link.Attrs().Name)
	event := &Event{
		EventType: v1.EventTypeWarning,
		Reason:    "atypical deletion",
		Message:   message,
	}

	if v.helper != nil && v.helper.EventSender != nil {
		if err := v.helper.EventSender(event); err != nil {
			klog.Errorf("recorde event failed, error: %s", err.Error())
		}
	}

	if v.helper != nil && v.helper.SendResetSignal != nil {
		if err := v.helper.SendResetSignal(); err != nil {
			klog.Errorf("reset vlan failed, error: %s", err.Error())
		}
	}
}

func (v *Vlan) afterModifyNicIP(addr netlink.AddrUpdate) {
	message := fmt.Sprintf("The IP address of %s has been modified as %s and the bridge %s will keep the same",
		v.nic.Name(), addr.LinkAddress.String(), v.bridge.Name())
	event := &Event{
		EventType: v1.EventTypeNormal,
		Reason:    "IPv4AddressUpdate",
		Message:   message,
	}

	if v.helper != nil && v.helper.EventSender != nil {
		if err := v.helper.EventSender(event); err != nil {
			klog.Errorf("recorde event failed, error: %s", err.Error())
		}
	}

	if err := v.bridge.ConfigIPv4AddrFromSlave(v.nic, nil); err != nil {
		klog.Errorf("configure bridge ip failed, error: %s", err.Error())
	}
}

func (v *Vlan) afterLinkDown(update netlink.LinkUpdate) {
	if update.Link.Attrs().OperState == netlink.OperDown && (update.Link.Attrs().Flags&net.FlagUp) == 0 && update.Change == 1 {
		event := &Event{
			EventType: v1.EventTypeWarning,
			Reason:    "LinkDown",
			Message:   fmt.Sprintf("Link %s has been down atypically and the controller will try to set it up", update.Link.Attrs().Name),
		}

		if v.helper != nil && v.helper.EventSender != nil {
			if err := v.helper.EventSender(event); err != nil {
				klog.Errorf("recorde event failed, error: %s", err.Error())
			}
		}

		if err := netlink.LinkSetUp(update.Link); err != nil {
			klog.Errorf("link %s set up failed, error: %s", update.Link.Attrs().Name, err.Error())
		}
	}
}
