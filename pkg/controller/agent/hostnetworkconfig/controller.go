package hostnetworkconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"github.com/vishvananda/netlink"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/network/vlan"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ControllerName = "harvester-network-hostnetworkconfig-controller"
	IPModeDHCP     = "dhcp"
	IPModeStatic   = "static"
)

type Handler struct {
	nodeName          string
	nodeClient        ctlcorev1.NodeClient
	nodeCache         ctlcorev1.NodeCache
	hostNetworkClient ctlnetworkv1.HostNetworkConfigClient
	hostNetworkCache  ctlnetworkv1.HostNetworkConfigCache
	cnCache           ctlnetworkv1.ClusterNetworkCache
	cnController      ctlnetworkv1.ClusterNetworkController

	mu            sync.Mutex
	leaseManagers map[string]*LeaseManager
	mgmtIntfName  string
}

func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	hns := management.HarvesterNetworkFactory.Network().V1beta1().HostNetworkConfig()
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()
	var mgmtIntf string
	var err error

	handler := &Handler{
		nodeName:          management.Options.NodeName,
		nodeClient:        nodes,
		nodeCache:         nodes.Cache(),
		hostNetworkClient: hns,
		hostNetworkCache:  hns.Cache(),
		cnCache:           cns.Cache(),
		cnController:      cns,
		leaseManagers:     make(map[string]*LeaseManager),
	}

	if mgmtIntf, err = iface.GetMgmtInterface(); err != nil {
		return fmt.Errorf("failed to get management interface for node %s, error: %w", handler.nodeName, err)
	}
	handler.mgmtIntfName = mgmtIntf

	hns.OnChange(ctx, ControllerName, handler.OnChange)
	hns.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

func checkifHostNetworkInterfaceExists(hnc *networkv1.HostNetworkConfig) (bool, error) {
	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			return false, nil
		}
		return false, err
	}

	bridgelink, err := v.GetBridgelink()
	if err != nil {
		return false, err
	}

	vlanIntf := utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID)
	_, err = netlink.LinkByName(vlanIntf)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (h *Handler) OnChange(_ string, hnc *networkv1.HostNetworkConfig) (*networkv1.HostNetworkConfig, error) {
	if hnc == nil || hnc.DeletionTimestamp != nil {
		return nil, nil
	}

	logrus.Infof("hostnetwork config %s is changed, spec: %+v", hnc.Name, hnc.Spec)

	matchNodeSet, err := h.matchNode(hnc.Spec.NodeSelector)
	if err != nil {
		return nil, err
	}

	intfExists, err := checkifHostNetworkInterfaceExists(hnc)
	if err != nil {
		return nil, err
	}

	// node selector doesn't match, need to clean up the host network config if exists
	if !matchNodeSet {
		if intfExists {
			return h.removeHostNetworkInterface(hnc, true)
		}

		// always ensure the status is cleaned
		err := h.removeHostNetworkPerNodeStatus(hnc)
		if err != nil {
			return nil, err
		}
		return hnc, nil
	}

	// node selector matches and host network interface already exists, skip processing
	if intfExists {
		logrus.Infof("hostnetwork config %s has been applied on this node already, update nodestatus and skip", hnc.Name)
		err := h.setHostNetworkPerNodeStatus(hnc, true, nil)
		if err != nil {
			return nil, err
		}
		return hnc, nil
	}

	var addr string
	var bridgelink *iface.Link

	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			logrus.Infof("cluster network %s is not set on this node, skip", hnc.Spec.ClusterNetwork)
			//stop and delete all lease manaagers assosciated with the cluster network (if uplink removed due to vlanconfig changes/deletion)
			h.stopLeaseManager(utils.GetClusterNetworkVlanDevice(hnc.Spec.ClusterNetwork, hnc.Spec.VlanID))
			return nil, nil
		}
		return hnc, h.updateHostNetworkReadyStatus(hnc, err)
	}

	bridgelink, err = v.GetBridgelink()
	if err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, err)
	}

	if err = bridgelink.AddBridgeVlanSelf(hnc.Spec.VlanID); err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, err)
	}

	if err = bridgelink.CreateVlanSubInterface(hnc.Spec.VlanID); err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, err)
	}

	// reconcile cluster network to add vid to the uplink(cluster-bo)
	if err := h.wakeUpClusterNetwork(hnc.Spec.ClusterNetwork); err != nil {
		return nil, fmt.Errorf("wake up cluster network %s failed, error: %w", hnc.Spec.ClusterNetwork, err)
	}

	switch hnc.Spec.Mode {
	case IPModeDHCP:
		if err = h.startLeaseManager(bridgelink, hnc.Spec.VlanID); err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, err)
		}

	case IPModeStatic:
		// stop lease manager if exists (previously in dhcp mode)
		h.stopLeaseManager(utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID))

		if addr, err = findMatchingIPfromNode(h.nodeName, hnc.Spec.HostIPs); err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, err)
		}

		if err := bridgelink.SetIPAddress(addr, hnc.Spec.VlanID); err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, err)
		}
	default:
		err = fmt.Errorf("unsupported ip assignment mode %s for host network config %s", hnc.Spec.Mode, hnc.Name)
		return hnc, h.updateHostNetworkReadyStatus(hnc, err)
	}

	//success case, update host network config status to ready
	if updateErr := h.updateHostNetworkReadyStatus(hnc, nil); updateErr != nil {
		return hnc, updateErr
	}

	// update node annotation to set the vlan sub interface to be used as underlay (if underlay is enabled)
	// and set to default mgmt interface if underlay is not enabled
	if err := h.addNodeAnnotation(utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID), hnc.Spec.Underlay); err != nil {
		return nil, fmt.Errorf("add node annotation to node %s for host network config %s failed, error: %w", h.nodeName, hnc.Name, err)
	}

	return hnc, nil
}

func (h *Handler) updateHostNetworkReadyStatus(hnc *networkv1.HostNetworkConfig, l3setupErr error) error {
	if statusUpdateErr := h.setHostNetworkStatus(hnc, l3setupErr); statusUpdateErr != nil {
		return fmt.Errorf("set host network %s unready failed, error: %w configErr: %v", hnc.Name, statusUpdateErr, l3setupErr)
	}

	if l3setupErr != nil {
		return fmt.Errorf("setup host network config %s failed, error: %w", hnc.Name, l3setupErr)
	}

	return nil
}

func (h *Handler) removeHostNetworkInterface(hnc *networkv1.HostNetworkConfig, onChange bool) (*networkv1.HostNetworkConfig, error) {
	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			logrus.Infof("cluster network %s is not set on this node, skip", hnc.Spec.ClusterNetwork)
			return nil, nil
		}
		return nil, err
	}

	bridgelink, err := v.GetBridgelink()
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			return nil, nil
		} else {
			return nil, fmt.Errorf("failed to get link for bridge %s, error: %w", v.Bridge().Name, err)
		}
	}

	if err := bridgelink.DelBridgeVlanSelf(hnc.Spec.VlanID); err != nil {
		return nil, fmt.Errorf("del bridge vlanconfig %d failed for %s, error: %w", hnc.Spec.VlanID, v.Bridge().Name, err)
	}

	h.stopLeaseManager(utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID))

	if err := bridgelink.DelVlanSubInterface(hnc.Spec.VlanID); err != nil {
		return nil, fmt.Errorf("del vlan subinterface %d failed for %s, error: %w", hnc.Spec.VlanID, v.Bridge().Name, err)
	}

	// reconcile cluster network to delete vid from the uplink(cluster-bo)
	if err := h.wakeUpClusterNetwork(hnc.Spec.ClusterNetwork); err != nil {
		return nil, fmt.Errorf("wake up cluster network %s failed, error: %w", hnc.Spec.ClusterNetwork, err)
	}

	//update nodestatus when interface deleted due to node selector changes.
	if onChange {
		if err := h.removeHostNetworkPerNodeStatus(hnc); err != nil {
			return nil, err
		}
	}

	return hnc, nil
}

func (h *Handler) OnRemove(_ string, hnc *networkv1.HostNetworkConfig) (*networkv1.HostNetworkConfig, error) {
	if hnc == nil {
		return nil, nil
	}

	logrus.Infof("hostnetwork config %s has been removed, spec: %+v", hnc.Name, hnc.Spec)

	return h.removeHostNetworkInterface(hnc, false)
}

// reconcile cluster network to add/delete vid to the uplink(cluster-bo) after hostnetworkconfig changes
func (h *Handler) wakeUpClusterNetwork(clusterNetwork string) error {
	_, err := h.cnCache.Get(clusterNetwork)
	if err == nil {
		h.cnController.Enqueue(clusterNetwork)
		return nil
	}

	return err
}

func findMatchingIPfromNode(nodeName string, hostIPs map[string]networkv1.IPAddr) (string, error) {
	addr := hostIPs[nodeName]
	if addr != "" {
		return string(addr), nil
	}

	// if no matching IP found for the node, return error to set host network config status to not ready
	return "", fmt.Errorf("no matching IP found for node %s", nodeName)
}

func (h *Handler) removeHostNetworkPerNodeStatus(hnc *networkv1.HostNetworkConfig) error {
	if hnc.Status.NodeStatus == nil {
		return nil
	}

	if _, exists := hnc.Status.NodeStatus[h.nodeName]; !exists {
		return nil
	}

	patchPayload := map[string]interface{}{
		"status": map[string]interface{}{
			"nodeStatus": map[string]interface{}{
				h.nodeName: nil,
			},
		},
	}

	patchBytes, err := json.Marshal(patchPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = h.hostNetworkClient.Patch(
		hnc.Name,
		types.MergePatchType,
		patchBytes,
		"status",
	)
	if err != nil {
		return fmt.Errorf("failed to patch HostNetworkConfig status for node %s: %w", h.nodeName, err)
	}

	return nil
}

func (h *Handler) setHostNetworkPerNodeStatus(hnc *networkv1.HostNetworkConfig, ready bool, setupErr error) error {
	patchPayload := map[string]interface{}{
		"status": map[string]interface{}{
			"nodeStatus": map[string]interface{}{
				h.nodeName: map[string]interface{}{
					"clusterNetwork": hnc.Spec.ClusterNetwork,
					"vlanID":         hnc.Spec.VlanID,
					"mode":           hnc.Spec.Mode,
					"conditions": []map[string]interface{}{
						{
							"type": networkv1.Ready,
							"status": func() string {
								if ready {
									return "True"
								} else {
									return "False"
								}
							}(),
							"message": func() string {
								if ready {
									return ""
								} else {
									return fmt.Sprintf("setup l3 connectivity failed: %v", setupErr)
								}
							}(),
						},
					},
				},
			},
		},
	}

	patchBytes, err := json.Marshal(patchPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = h.hostNetworkClient.Patch(
		hnc.Name,
		types.MergePatchType,
		patchBytes,
		"status",
	)
	if err != nil {
		return fmt.Errorf("failed to patch HostNetworkConfig status for node %s: %w", h.nodeName, err)
	}

	return nil
}

func (h *Handler) setHostNetworkStatus(hnc *networkv1.HostNetworkConfig, setupErr error) error {
	if setupErr != nil {
		return h.setHostNetworkPerNodeStatus(hnc, false, setupErr)
	} else {
		return h.setHostNetworkPerNodeStatus(hnc, true, nil)
	}
}

func (h *Handler) stopLeaseManager(vlanIntfName string) {
	h.mu.Lock()
	lm := h.leaseManagers[vlanIntfName]
	if lm != nil {
		delete(h.leaseManagers, vlanIntfName)
	}
	h.mu.Unlock()

	if lm == nil {
		return
	}
	lm.Stop()
}

func (h *Handler) getOrCreateLeaseManager(bridgelink *iface.Link, vlanID uint16) (*LeaseManager, error) {
	vlanIntfName := utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, vlanID)

	h.mu.Lock()
	lm := h.leaseManagers[vlanIntfName]
	h.mu.Unlock()

	if lm != nil {
		return lm, nil
	}

	newLM, err := NewLeaseManager(vlanIntfName, bridgelink, vlanID)
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.leaseManagers[vlanIntfName] = newLM

	return newLM, nil
}

func (h *Handler) startLeaseManager(bridgelink *iface.Link, vlanID uint16) (err error) {
	lm, err := h.getOrCreateLeaseManager(bridgelink, vlanID)
	if err != nil {
		return err
	}

	if err := lm.Start(context.Background()); err != nil {
		return err
	}

	return nil
}

func (h *Handler) addNodeAnnotation(underlayIntfName string, underlay bool) error {
	node, err := h.nodeCache.Get(h.nodeName)
	if err != nil {
		return err
	}

	if node == nil {
		return fmt.Errorf("node %s not found", h.nodeName)
	}

	if node.DeletionTimestamp != nil {
		return nil
	}

	//set to default mgmt interface if underlay is not enabled
	if !underlay {
		underlayIntfName = h.mgmtIntfName
	}

	if node.Annotations != nil && node.Annotations[utils.KeyUnderlayIntf] == underlayIntfName {
		return nil
	}

	nodeCopy := node.DeepCopy()
	if nodeCopy.Annotations == nil {
		nodeCopy.Annotations = make(map[string]string)
	}

	nodeCopy.Annotations[utils.KeyUnderlayIntf] = underlayIntfName

	if _, err := h.nodeClient.Update(nodeCopy); err != nil {
		return fmt.Errorf("failed to update tunnel interface %s annotation to node %s, error %w", underlayIntfName, h.nodeName, err)
	}

	return nil
}

func (h *Handler) matchNode(nodeSelector *metav1.LabelSelector) (bool, error) {
	// if node selector is nil, all nodes match by default, return true
	if nodeSelector == nil {
		return true, nil
	}

	node, err := h.nodeCache.Get(h.nodeName)
	if err != nil {
		return false, err
	}

	if node == nil {
		return false, fmt.Errorf("node %s not found", h.nodeName)
	}

	if node.DeletionTimestamp != nil {
		return false, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(nodeSelector)
	if err != nil {
		return false, err
	}

	if !selector.Matches(labels.Set(node.Labels)) {
		// node doesn't match the selector, skip processing
		return false, nil
	}

	return true, nil
}
