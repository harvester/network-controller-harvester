package hostnetwork

import (
	"context"
	"fmt"
	"os"
	"time"

	ctlharv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	networkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/apis/network.harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/common"
	"github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester-network-controller/pkg/network"
	"github.com/rancher/harvester-network-controller/pkg/network/iface"
	"github.com/rancher/harvester-network-controller/pkg/network/vlan"

	cniv1 "github.com/rancher/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
)

const (
	controllerName = "hostnetwork-controller"
	bridgeCNIName  = "bridge"
	resetPeriod    = time.Minute * 30
)

type Handler struct {
	hostNetworkCtr   v1alpha1.HostNetworkController
	hostNetworkCache v1alpha1.HostNetworkCache
	settingCache     ctlharv1.SettingCache
	nadCache         cniv1.NetworkAttachmentDefinitionCache
	recorder         record.EventRecorder
}

func Register(ctx context.Context, management *config.Management) error {
	hns := management.HarvesterNetworkFactory.Network().V1alpha1().HostNetwork()
	settings := management.HarvesterFactory.Harvester().V1alpha1().Setting()
	nad := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()

	handler := &Handler{
		hostNetworkCtr:   hns,
		hostNetworkCache: hns.Cache(),
		settingCache:     settings.Cache(),
		nadCache:         nad.Cache(),
		recorder:         management.NewRecorder(controllerName, "", ""),
	}

	hns.OnChange(ctx, controllerName, handler.OnChange)
	hns.OnRemove(ctx, controllerName, handler.OnRemove)

	// start network monitoring
	go network.GetWatcher().Start(context.TODO())

	// regular reset host network
	go func() {
		ticker := time.NewTicker(resetPeriod)
		for range ticker.C {
			klog.Infof("regular reset host network")
			if err := handler.resetHostNetwork(); err != nil {
				klog.Errorf("regular reset vlan network failed")
			}
		}
	}()

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
		if err := h.removeVlanNetwork(hn); err != nil {
			return nil, err
		}
	default:
	}

	return hn, nil
}

func (h Handler) configVlanNetwork(hn *networkv1alpha1.HostNetwork) error {
	nic, err := common.GetNIC(hn.Spec.NIC, h.settingCache)
	if err != nil {
		return fmt.Errorf("get nic failed, error: %w", err)
	}

	klog.Infof("NIC: %s", nic)

	h.repealVlan(hn, nic)

	return h.setupVlan(hn, nic)
}

func (h Handler) setupVlan(hn *networkv1alpha1.HostNetwork, nic string) error {
	if nic == "" {
		return h.updateStatus(hn, &vlan.Status{
			Condition: vlan.Condition{Normal: false, Message: "The physical NIC for VLAN network hasn't configured yet"},
		})
	}

	v := vlan.NewVlan(&vlan.Helper{EventSender: h.sendEvent, Resetter: h.resetHostNetwork})
	routes := getRoutes(vlan.BridgeName, hn)

	vids, err := h.getNadVidList()
	if err != nil {
		return err
	}

	if err = v.Setup(nic, vids, network.Config{Routes: routes}); err != nil {
		err = fmt.Errorf("set up vlan failed, error: %w, nic: %s", err, nic)
		return h.updateStatus(hn, &vlan.Status{
			Condition: vlan.Condition{Normal: false, Message: err.Error()},
		})
	}

	status, err := v.Status(vlan.Condition{Normal: true, Message: ""})
	if err != nil {
		return fmt.Errorf("get status failed, error: %w", err)
	}
	return h.updateStatus(hn, status)
}

func (h Handler) repealVlan(hn *networkv1alpha1.HostNetwork, nic string) {
	configuredNic := h.getNICFromStatus(hn)
	klog.Infof("configuredNIC: %s", configuredNic)

	// try to repeal VLAN network with previous configured NIC
	if configuredNic != "" && configuredNic != nic {
		v, err := vlan.GetVlanWithNic(configuredNic, nil)
		if err != nil {
			klog.Warningf("create vlan with nic %s failed, error: %s", configuredNic, err.Error())
		} else {
			if err := v.Repeal(); err != nil {
				klog.Warningf("repeal vlan failed, error: %s, nic: %s", err.Error(), configuredNic)
			}
		}
	}
}

// It's a callback function for pkg/network/vlan to help to reset vlan network.
// HostNetwork is unknown when function is called inside pkg/network/vlan package,
// so we use hostNetworkCache to get hostNetwork.
func (h Handler) resetHostNetwork() error {
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

func (h Handler) removeVlanNetwork(hn *networkv1alpha1.HostNetwork) error {
	nic, err := common.GetNIC(hn.Spec.NIC, h.settingCache)
	if err != nil {
		return fmt.Errorf("get nic failed, error: %w", err)
	}

	v, err := vlan.GetVlanWithNic(nic, nil)
	if err != nil {
		return fmt.Errorf("new vlan with nic %s failed, error: %w", nic, err)
	}

	if err := v.Repeal(); err != nil {
		return fmt.Errorf("repeal vlan failed, error: %w", err)
	}

	return nil
}

func (h Handler) updateStatus(hn *networkv1alpha1.HostNetwork, status *vlan.Status) error {
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

	// get all physical NICs
	nics, err := getPhysicalNics()
	if err != nil {
		return err
	}
	hnCopy.Status.PhysicalNics = nics

	networkv1alpha1.HostNetworkReady.SetStatusBool(hnCopy, status.Condition.Normal)
	networkv1alpha1.HostNetworkReady.Message(hnCopy, status.Condition.Message)

	if _, err := h.hostNetworkCtr.UpdateStatus(hnCopy); err != nil {
		return fmt.Errorf("update status of hostnetwork %s failed, error: %w", hn.Name, err)
	}

	return nil
}

func (h Handler) getNadVidList() ([]uint16, error) {
	nads, err := h.nadCache.List("", labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list nad failed, error: %v", err)
	}

	vidList := make([]uint16, 0)

	for _, nad := range nads {
		conf, err := common.DecodeNetConf(nad.Spec.Config)
		if err != nil {
			return nil, err
		}

		if conf.Type == bridgeCNIName && conf.BrName == vlan.BridgeName {
			klog.Infof("add nad:%s with vid:%d to the list", nad.Name, conf.Vlan)
			vidList = append(vidList, uint16(conf.Vlan))
		}
	}

	return vidList, nil
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

func getPhysicalNics() ([]networkv1alpha1.PhysicalNic, error) {
	nics, err := iface.GetPhysicalNics()
	if err != nil {
		return nil, fmt.Errorf("list physical NICs failed")
	}

	physicalNics := []networkv1alpha1.PhysicalNic{}
	for index, nic := range nics {
		physicalNics = append(physicalNics, networkv1alpha1.PhysicalNic{
			Index:               index,
			Name:                nic.Name,
			UsedByManageNetwork: nic.UsedByManageNetwork,
		})
	}

	return physicalNics, nil
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
