package nodenetwork

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	harvnetwork "github.com/harvester/harvester/pkg/api/network"
	cniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"github.com/vishvananda/netlink"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/common"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/network/mgmt"
	"github.com/harvester/harvester-network-controller/pkg/network/vlan"
)

// NodeNetwork controller watches NodeNetwork to configure network for cluster node
const (
	controllerName = "harvester-nodenetwork-controller"
	resetPeriod    = time.Minute * 30

	bridgeCNIName = "bridge"
)

type Handler struct {
	nodeNetworkCtr   ctlnetworkv1.NodeNetworkController
	nodeNetworkCache ctlnetworkv1.NodeNetworkCache
	nadCache         cniv1.NetworkAttachmentDefinitionCache
	recorder         record.EventRecorder
	mgmtNetwork      network.Network
}

func Register(ctx context.Context, management *config.Management) error {
	nns := management.HarvesterNetworkFactory.Network().V1beta1().NodeNetwork()
	nad := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()

	handler := &Handler{
		nodeNetworkCtr:   nns,
		nodeNetworkCache: nns.Cache(),
		nadCache:         nad.Cache(),
		recorder:         management.NewRecorder(controllerName, "", ""),
	}

	switch management.Options.MgmtNetworkType {
	case "flannel":
		mgmtNetwork, err := mgmt.NewFlannelNetwork(management.Options.MgmtNetworkDevice)
		if err != nil {
			return err
		}
		handler.mgmtNetwork = mgmtNetwork
	}

	nns.OnChange(ctx, controllerName, handler.OnChange)
	nns.OnRemove(ctx, controllerName, handler.OnRemove)

	// start network monitoring
	go network.GetWatcher().Start(ctx)

	// regular reset node network
	go func() {
		ticker := time.NewTicker(resetPeriod)
		for range ticker.C {
			klog.Infof("regular reset node network")
			if err := handler.reconcileNodeNetwork(string(networkv1.NetworkTypeVLAN)); err != nil {
				klog.Errorf("regular reset vlan network failed, error: %+v", err)
			}
		}
	}()

	return nil
}

func (h Handler) OnChange(key string, nn *networkv1.NodeNetwork) (*networkv1.NodeNetwork, error) {
	if nn == nil || nn.DeletionTimestamp != nil {
		return nil, nil
	}

	nodeName := os.Getenv(common.KeyNodeName)
	if nn.Spec.NodeName != nodeName {
		return nil, nil
	}

	klog.Infof("node network configuration %s has been changed, spec: %+v", nn.Name, nn.Spec)

	switch nn.Spec.Type {
	case networkv1.NetworkTypeVLAN:
		if err := h.configVlanNetwork(nn); err != nil {
			return nil, err
		}
	default:
	}

	return nn, nil
}

func (h Handler) OnRemove(key string, nn *networkv1.NodeNetwork) (*networkv1.NodeNetwork, error) {
	if nn == nil {
		return nil, nil
	}
	nodeName := os.Getenv(common.KeyNodeName)
	if nn.Spec.NodeName != nodeName {
		return nil, nil
	}
	klog.Infof("node network configuration %s has been deleted", nn.Name)

	switch nn.Spec.Type {
	case networkv1.NetworkTypeVLAN:
		if err := h.removeVlanNetwork(); err != nil {
			return nil, err
		}
	default:
	}

	return nn, nil
}

func (h Handler) configVlanNetwork(nn *networkv1.NodeNetwork) error {
	if err := h.repealVlan(nn); err != nil {
		return err
	}

	return h.setupVlan(nn)
}

func (h Handler) setupVlan(nn *networkv1.NodeNetwork) error {
	if nn.Spec.NIC == "" {
		return h.updateStatus(nn, network.Status{
			Condition: network.Condition{Normal: false, Message: "A physical NIC has not been specified yet"},
		})
	}
	vids, err := h.getNadVidList()
	if err != nil {
		return err
	}

	v := vlan.NewVlan(h, h.mgmtNetwork, vids)

	if err = v.Setup(nn.Spec.NIC); err != nil {
		if statusErr := h.updateStatus(nn, network.Status{
			Condition: network.Condition{Normal: false, Message: "Setup VLAN network failed, please try another NIC"},
		}); statusErr != nil {
			return statusErr
		}
		return fmt.Errorf("set up vlan failed, error: %w, nic: %s", err, nn.Spec.NIC)
	}

	status, err := v.Status(network.Condition{Normal: true, Message: ""})
	if err != nil {
		return fmt.Errorf("get status failed, error: %w", err)
	}
	return h.updateStatus(nn, *status)
}

func (h Handler) repealVlan(nn *networkv1.NodeNetwork) error {
	v, err := vlan.GetVlan()
	if err != nil && !errors.As(err, &netlink.LinkNotFoundError{}) && !errors.As(err, &vlan.SlaveNotFoundError{}) {
		return err
	} else if err != nil {
		klog.Infof("ignore link not found error, details: %+v", err)
		return nil
	}

	configuredNIC := v.NIC().Name()
	if configuredNIC != "" && configuredNIC != nn.Spec.NIC {
		if err := v.Teardown(); err != nil {
			return fmt.Errorf("tear down vlan failed, error: %s, nic: %s", err.Error(), configuredNIC)
		}
	}

	return nil
}

// It's a callback function for pkg/network/vlan to help to reset vlan network.
// NodeNetwork is unknown when function is called inside pkg/network/vlan package,
// so we use nodeNetworkCache to get nodeNetwork.
func (h Handler) reconcileNodeNetwork(networkType string) error {
	name := os.Getenv(common.KeyNodeName) + "-" + networkType
	nn, err := h.nodeNetworkCache.Get(common.Namespace, name)
	if err != nil {
		return fmt.Errorf("get nn %s failed, error: %w", name, err)
	}

	h.nodeNetworkCtr.Enqueue(common.Namespace, nn.Name)

	return nil
}

func (h Handler) getNICFromStatus(nn *networkv1.NodeNetwork) string {
	for linkName, status := range nn.Status.NetworkLinkStatus {
		if status.Type == "device" || status.Type == "bond" {
			return linkName
		}
	}

	return ""
}

func (h Handler) removeVlanNetwork() error {
	v, err := vlan.GetVlan()
	if err != nil && !errors.As(err, &netlink.LinkNotFoundError{}) && !errors.As(err, &vlan.SlaveNotFoundError{}) {
		return err
	} else if err != nil {
		klog.Infof("ignore link not found error, details: %+v", err)
		return nil
	}

	if err := v.Teardown(); err != nil {
		return fmt.Errorf("tear down vlan failed, error: %w", err)
	}

	return nil
}

func (h Handler) updateStatus(nn *networkv1.NodeNetwork, status network.Status) error {
	nnCopy := nn.DeepCopy()

	for name := range nn.Status.NetworkLinkStatus {
		if _, ok := status.IFaces[name]; !ok {
			delete(nnCopy.Status.NetworkLinkStatus, name)
		}
	}

	if nnCopy.Status.NetworkLinkStatus == nil {
		nnCopy.Status.NetworkLinkStatus = make(map[string]*networkv1.LinkStatus)
	}

	for name, link := range status.IFaces {
		nnCopy.Status.NetworkLinkStatus[name] = makeLinkStatus(link)
	}

	// get all physical NICs
	nics, err := h.getNICs()
	if err != nil {
		return err
	}
	nnCopy.Status.NICs = nics

	networkv1.NodeNetworkReady.SetStatusBool(nnCopy, status.Condition.Normal)
	networkv1.NodeNetworkReady.Message(nnCopy, status.Condition.Message)

	if _, err := h.nodeNetworkCtr.Update(nnCopy); err != nil {
		return fmt.Errorf("update status of nodenetwork %s failed, error: %w", nn.Name, err)
	}

	return nil
}

func (h Handler) getNadVidList() ([]uint16, error) {
	nads, err := h.nadCache.List("", labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("list nad failed, error: %v", err)
	}

	vidList := make([]uint16, 0)

	for _, n := range nads {
		netconf := &harvnetwork.NetConf{}
		if err := json.Unmarshal([]byte(n.Spec.Config), netconf); err != nil {
			return nil, fmt.Errorf("unmarshal failed, error: %w, value: %s", err, n.Spec.Config)
		}

		if netconf.Type == bridgeCNIName && netconf.BrName == vlan.BridgeName {
			klog.Infof("add nad:%s with vid:%d to the list", n.Name, netconf.Vlan)
			vidList = append(vidList, uint16(netconf.Vlan))
		}
	}

	return vidList, nil
}

func makeLinkStatus(link iface.IFace) *networkv1.LinkStatus {
	linkStatus := &networkv1.LinkStatus{
		Index:       link.Index(),
		MasterIndex: link.LinkAttrs().MasterIndex,
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

func (h Handler) getNICs() ([]networkv1.NIC, error) {
	links, err := iface.ListLinks(map[string]bool{iface.TypeDevice: true, iface.TypeBond: true})
	if err != nil {
		return nil, fmt.Errorf("list physical NICs failed")
	}

	mgmtNICIndex := h.mgmtNetwork.NIC().Index()

	nics := []networkv1.NIC{}
	for _, l := range links {
		nic := networkv1.NIC{
			Index:       l.Index(),
			MasterIndex: l.LinkAttrs().MasterIndex,
			Name:        l.Name(),
			Type:        l.Type(),
			State:       l.LinkAttrs().OperState.String(),
		}
		if l.Index() == mgmtNICIndex {
			nic.UsedByMgmtNetwork = true
		}

		nics = append(nics, nic)
	}

	return nics, nil
}

func (h Handler) SendEvent(e *network.Event, networkType string) error {
	name := os.Getenv(common.KeyNodeName)
	nn, err := h.nodeNetworkCache.Get(common.Namespace, name+"-"+networkType)
	if err != nil {
		return err
	}
	ref := &corev1.ObjectReference{
		Name:       nn.Name,
		Namespace:  nn.Namespace,
		UID:        nn.UID,
		Kind:       nn.Kind,
		APIVersion: nn.APIVersion,
	}
	h.recorder.Event(ref, e.EventType, e.Reason, e.Message)

	return nil
}
