package hostnetwork

import (
	"context"
	"fmt"
	"os"

	ctlharv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	networkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/apis/network.harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/common"
	"github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester-network-controller/pkg/network"
	"github.com/rancher/harvester-network-controller/pkg/network/iface"
	"github.com/rancher/harvester-network-controller/pkg/network/vlan"
)

const (
	controllerName = "harvester-network-hostnetwork-controller"
)

type Handler struct {
	hostNetworkCtr   v1alpha1.HostNetworkController
	hostNetworkCache v1alpha1.HostNetworkCache
	settingCache     ctlharv1.SettingCache
	recorder         record.EventRecorder
}

func Register(ctx context.Context, management *config.Management) error {
	hns := management.HarvesterNetworkFactory.Network().V1alpha1().HostNetwork()
	settings := management.HarvesterFactory.Harvester().V1alpha1().Setting()

	handler := &Handler{
		hostNetworkCtr:   hns,
		hostNetworkCache: hns.Cache(),
		settingCache:     settings.Cache(),
		recorder:         management.NewRecorder(controllerName, "", ""),
	}

	hns.OnChange(ctx, controllerName, handler.OnChange)
	hns.OnRemove(ctx, controllerName, handler.OnRemove)

	// start network monitoring
	go network.GetWatcher().Start(context.TODO())

	return nil
}

func (h Handler) OnChange(key string, hn *networkv1alpha1.HostNetwork) (*networkv1alpha1.HostNetwork, error) {
	if hn == nil || hn.DeletionTimestamp != nil {
		return nil, nil
	}

	hostname := os.Getenv(common.KeyHostName)
	if hn.Name != hostname {
		return nil, nil
	}

	klog.Infof("host network configuration %s has been changed, spec: %+v", hn.Name, hn.Spec)

	switch hn.Spec.Type {
	case networkv1alpha1.NetworkTypeVLAN:
		if err := h.configVlanNetwork(hn); err != nil {
			return nil, err
		}
	default:
	}

	return hn, nil
}

func (h Handler) OnRemove(key string, hn *networkv1alpha1.HostNetwork) (*networkv1alpha1.HostNetwork, error) {
	if hn == nil {
		return nil, nil
	}
	hostname := os.Getenv(common.KeyHostName)
	if hn.Name != hostname {
		return nil, nil
	}
	klog.Infof("host network configuration %s has been deleted", hn.Name)

	switch hn.Spec.Type {
	case networkv1alpha1.NetworkTypeVLAN:
		if err := h.repealVlanNetwork(); err != nil {
			return nil, err
		}
	default:
	}

	return hn, nil
}

func (h Handler) configVlanNetwork(hn *networkv1alpha1.HostNetwork) error {
	configuredNic := h.getNICFromStatus(hn)
	nic, err := common.GetNIC(h.hostNetworkCache, h.settingCache)
	if err != nil {
		return fmt.Errorf("get nic failed, error: %w", err)
	}

	klog.Infof("NIC: %s, configuredNIC: %s", nic, configuredNic)
	if configuredNic != "" && configuredNic != nic {
		preVlan, err := vlan.NewVlanWithNic(configuredNic, nil)
		if err != nil {
			klog.Warningf("create vlan with nic %s failed, error: %s", configuredNic, err.Error())
		} else {
			if err := preVlan.Repeal(); err != nil {
				klog.Warningf("repeal vlan failed, error: %s, nic: %s", err.Error(), configuredNic)
			}
		}
	}

	if nic == "" {
		return h.updateStatus(&vlan.Status{
			Condition: vlan.Condition{Normal: true, Message: "The physical NIC for VLAN network hasn't configured yet"},
		})
	}

	curVlan := vlan.NewPureVlan(&vlan.Helper{EventSender: h.sendEvent, SendResetSignal: h.resetVlanNetwork})
	condition := vlan.Condition{Normal: true, Message: ""}
	routes := getRoutes(vlan.BridgeName, hn)
	if err := curVlan.Setup(nic, network.Config{Routes: routes}); err != nil {
		err = fmt.Errorf("set up vlan failed, error: %w, nic: %s", err, nic)
		condition.Normal = false
		condition.Message = err.Error()
		if err := h.updateStatus(&vlan.Status{Condition: condition}); err != nil {
			klog.Errorf("update status failed, error: %s", err.Error())
		}
		return err
	}

	status, err := curVlan.Status(condition)
	if err != nil {
		return fmt.Errorf("get status failed, error: %w", err)
	}
	if err := h.updateStatus(status); err != nil {
		return fmt.Errorf("update status failed, error: %w", err)
	}

	return nil
}

func (h Handler) resetVlanNetwork() error {
	name := os.Getenv(common.KeyHostName)
	hn, err := h.hostNetworkCache.Get(common.HostNetworkNamespace, name)
	if err != nil {
		return fmt.Errorf("get host network %s failed, error: %w", name, err)
	}

	h.hostNetworkCtr.Enqueue(common.HostNetworkNamespace, hn.Name)

	return nil
}

func (h Handler) getNICFromStatus(hn *networkv1alpha1.HostNetwork) string {
	for linkName, status := range hn.Status.NetworkLinkStatus {
		if status.Type == "device" {
			return linkName
		}
	}

	return ""
}

func (h Handler) repealVlanNetwork() error {
	nic, err := common.GetNIC(h.hostNetworkCache, h.settingCache)
	if err != nil {
		return fmt.Errorf("get nic failed, error: %w", err)
	}

	v, err := vlan.NewVlanWithNic(nic, nil)
	if err != nil {
		return fmt.Errorf("new vlan with nic %s failed, error: %w", nic, err)
	}

	if err := v.Repeal(); err != nil {
		return fmt.Errorf("repeal vlan failed, error: %w", err)
	}

	return nil
}

func (h Handler) updateStatus(status *vlan.Status) error {
	name := os.Getenv(common.KeyHostName)
	hn, err := h.hostNetworkCache.Get(common.HostNetworkNamespace, name)
	if err != nil {
		return fmt.Errorf("get host network %s failed, error: %w", name, err)
	}

	hnCopy := hn.DeepCopy()

	for name := range hn.Status.NetworkLinkStatus {
		if _, ok := status.IFaces[name]; !ok {
			delete(hnCopy.Status.NetworkLinkStatus, name)
		}
	}

	if hnCopy.Status.NetworkLinkStatus == nil {
		hnCopy.Status.NetworkLinkStatus = make(map[string]*networkv1alpha1.LinkStatus)
	}

	for name, link := range status.IFaces {
		hnCopy.Status.NetworkLinkStatus[name] = makeLinkStatus(link)
	}

	networkv1alpha1.HostNetworkReady.SetStatusBool(hnCopy, status.Condition.Normal)
	networkv1alpha1.HostNetworkReady.Message(hnCopy, status.Condition.Message)

	_, err = h.hostNetworkCtr.UpdateStatus(hnCopy)
	if err != nil {
		return fmt.Errorf("update status of hostnetwork %s failed, error: %w", name, err)
	}

	return nil
}

func makeLinkStatus(link iface.IFace) *networkv1alpha1.LinkStatus {
	linkStatus := &networkv1alpha1.LinkStatus{
		Index:       link.Index(),
		Type:        link.Type(),
		MAC:         link.LinkAttrs().HardwareAddr.String(),
		Promiscuous: link.LinkAttrs().Promisc != 0,
		State:       link.LinkAttrs().OperState.String(),
	}

	for _, addr := range link.Addr() {
		linkStatus.IPV4Address = append(linkStatus.IPV4Address, addr.String())
	}
	for _, route := range link.Routes() {
		s, err := iface.Route2String(route)
		if err != nil {
			klog.Errorf("unmarshal route failed, route: %+v, error: %s", route, err.Error())
		} else {
			linkStatus.Routes = append(linkStatus.Routes, s)
		}
	}

	return linkStatus
}

func (h Handler) sendEvent(e *vlan.Event) error {
	name := os.Getenv(common.KeyHostName)
	hn, err := h.hostNetworkCache.Get(common.HostNetworkNamespace, name)
	if err != nil {
		return err
	}
	ref := &corev1.ObjectReference{
		Name:       hn.Name,
		Namespace:  hn.Namespace,
		UID:        hn.UID,
		Kind:       hn.Kind,
		APIVersion: hn.APIVersion,
	}
	h.recorder.Event(ref, e.EventType, e.Reason, e.Message)

	return nil
}

func getRoutes(link string, hn *networkv1alpha1.HostNetwork) []*netlink.Route {
	if hn.Status.NetworkLinkStatus == nil || hn.Status.NetworkLinkStatus[link] == nil {
		return nil
	}

	routes := []*netlink.Route{}
	for _, r := range hn.Status.NetworkLinkStatus[link].Routes {
		route, err := iface.String2Route(r)
		if err != nil {
			klog.Errorf("unmarshal route failed, route: %s, error: %s", route, err.Error())
			continue
		}
		routes = append(routes, route)
	}

	return routes
}
