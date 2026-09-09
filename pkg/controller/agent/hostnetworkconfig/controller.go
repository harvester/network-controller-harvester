package hostnetworkconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
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

// Standard Domain Sentinel Errors
var (
	ErrL2NotReady           = errors.New("l2 interface or bridge not ready")
	ErrL3NotReady           = errors.New("l3 address or dhcp allocation failed")
	ErrConfigurationInvalid = errors.New("invalid host network spec")
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

	stateMgr *LocalHostNetworkConfigStateManager
}

func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	hns := management.HarvesterNetworkFactory.Network().V1beta1().HostNetworkConfig()
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()
	var mgmtIntf string
	var err error

	ttl := getTTLFromEnvOrDefault()
	disabled := getDisableFromEnvOrDefault()

	handler := &Handler{
		nodeName:          management.Options.NodeName,
		nodeClient:        nodes,
		nodeCache:         nodes.Cache(),
		hostNetworkClient: hns,
		hostNetworkCache:  hns.Cache(),
		cnCache:           cns.Cache(),
		cnController:      cns,
		leaseManagers:     make(map[string]*LeaseManager),
		stateMgr:          NewLocalHostNetworkConfigStateManager(ttl, disabled),
	}

	if mgmtIntf, err = iface.GetMgmtInterface(); err != nil {
		return fmt.Errorf("failed to get management interface for node %s, error: %w", handler.nodeName, err)
	}
	handler.mgmtIntfName = mgmtIntf

	logrus.Infof("Node %s, mgmt interface %s, stateMgr initialized: %s", handler.nodeName, mgmtIntf, handler.stateMgr.String())

	hns.OnChange(ctx, ControllerName, handler.OnChange)
	hns.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

// OnChange handles creation, updates, and deletion events for HostNetworkConfig.
//
// Note: Although peer controllers may update the `utils.KeyMatchedNodes` annotation
// on HNC (triggering OnChange), this handler evaluates node matching dynamically
// via h.matchNode() rather than relying on that annotation.
//
// Hence, the local spec-based target hash remains robust: reconciliation state is
// strictly governed by the spec content paired with the local cache and CRD status checks
// (isAlreadyReady / isAlreadyRemoved) rather than dynamic metadata annotations.
func (h *Handler) OnChange(_ string, hnc *networkv1.HostNetworkConfig) (*networkv1.HostNetworkConfig, error) {
	if hnc == nil || hnc.DeletionTimestamp != nil {
		return nil, nil
	}

	logrus.Infof("hostnetwork config %s is changed, spec: %+v", hnc.Name, hnc.Spec)

	var targetHash string
	var err error
	// Skip hash computation when the state manager is disabled to save CPU cycles.
	// When targetHash remains empty (""), all downstream state checks (IsReady/IsRemoved)
	// immediately evaluate to false and safely fall back to full reconciliation.
	if !h.stateMgr.Disabled() {
		targetHash, err = utils.ComputeSpecHash(hnc.Spec)
		if err != nil {
			logrus.Debugf("failed to compute target spec hash for hostnetwork config %s, falling back to full process: %v", hnc.Name, err)
			targetHash = ""
		}
	}

	matchNodeSet, err := h.matchNode(hnc.Spec.NodeSelector)
	if err != nil {
		return nil, err
	}

	// node selector doesn't match, need to clean up the host network config if exists
	if !matchNodeSet {
		return h.handleNonMatchingNode(hnc, targetHash)
	}

	intfName := utils.GetClusterNetworkVlanDevice(hnc.Spec.ClusterNetwork, hnc.Spec.VlanID)

	// node selector matches, when fully ready, return quickly
	if h.isAlreadyReady(hnc, targetHash) {
		// update node annotation to set the vlan sub interface to be used as underlay (if underlay is enabled)
		// and set to default mgmt interface if underlay is not enabled
		if err := h.addNodeAnnotation(intfName, hnc.Spec.Underlay); err != nil {
			return nil, fmt.Errorf("add node annotation to node %s for host network config %s failed, error: %w", h.nodeName, hnc.Name, err)
		}

		logrus.Debugf("hostnetwork config %s is already setup and ready on node %s, fast exit", hnc.Name, h.nodeName)
		return hnc, nil
	}

	// run the setup process from the beginning
	var addr string
	var bridgelink *iface.Link

	// --- L2 Setup Phase ---
	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		// hand over to framework to retry
		logrus.Infof("cluster network %s is not set on this node, something might be wrong", hnc.Spec.ClusterNetwork)
		//stop and delete all lease manaagers assosciated with the cluster network (if uplink removed due to vlanconfig changes/deletion)
		// h.stopLeaseManager(utils.GetClusterNetworkVlanDevice(hnc.Spec.ClusterNetwork, hnc.Spec.VlanID))
		return hnc, h.updateHostNetworkReadyStatus(hnc, targetHash, err, ErrL2NotReady)
	}

	bridgelink, err = v.GetBridgelink()
	if err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, targetHash, err, ErrL2NotReady)
	}

	if err = bridgelink.AddBridgeVlanSelf(hnc.Spec.VlanID); err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, targetHash, err, ErrL2NotReady)
	}

	if err = bridgelink.CreateVlanSubInterface(hnc.Spec.VlanID); err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, targetHash, err, ErrL2NotReady)
	}

	// reconcile cluster network to add vid to the uplink(cluster-bo)
	if err := h.wakeUpClusterNetwork(hnc.Spec.ClusterNetwork); err != nil {
		return nil, fmt.Errorf("wake up cluster network %s failed, error: %w", hnc.Spec.ClusterNetwork, err)
	}

	// --- L3 Setup Phase ---
	switch hnc.Spec.Mode {
	case IPModeDHCP:
		if err = h.startLeaseManager(bridgelink, hnc.Spec.VlanID); err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, targetHash, err, ErrL3NotReady)
		}

	case IPModeStatic:
		// stop lease manager if exists (previously in dhcp mode)
		h.stopLeaseManager(utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID))

		if addr, err = findMatchingIPfromNode(h.nodeName, hnc.Spec.HostIPs); err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, targetHash, err, ErrL3NotReady)
		}

		if err := bridgelink.SetIPAddress(addr, hnc.Spec.VlanID); err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, targetHash, err, ErrL3NotReady)
		}
	default:
		err = fmt.Errorf("unsupported ip assignment mode %s for host network config %s", hnc.Spec.Mode, hnc.Name)
		return hnc, h.updateHostNetworkReadyStatus(hnc, targetHash, err, ErrConfigurationInvalid)
	}

	//success case, update host network config status to ready
	if updateErr := h.updateHostNetworkReadyStatus(hnc, targetHash, nil, nil); updateErr != nil {
		return hnc, updateErr
	}

	// update node annotation to set the vlan sub interface to be used as underlay (if underlay is enabled)
	// and set to default mgmt interface if underlay is not enabled
	if err := h.addNodeAnnotation(intfName, hnc.Spec.Underlay); err != nil {
		return nil, fmt.Errorf("add node annotation to node %s for host network config %s failed, error: %w", h.nodeName, hnc.Name, err)
	}

	return hnc, nil
}

// updateHostNetworkReadyStatus handles local cache state transitions and applies API status updates.
// 'l3setupErr' carries the detailed contextual error returned to the controller framework for logging and retry handling.
// 'categoryErr' specifies the high-level sentinel error (ErrL2NotReady, ErrL3NotReady, ErrConfigurationInvalid).
func (h *Handler) updateHostNetworkReadyStatus(hnc *networkv1.HostNetworkConfig, targetHash string, l3setupErr error, categoryErr error) error {
	if statusUpdateErr := h.setHostNetworkStatus(hnc, l3setupErr, categoryErr); statusUpdateErr != nil {
		// Mark cache as Unknown if status update fails
		h.stateMgr.CreateOrUpdate(hnc.Name, NewLocalHostNetworkConfigState(targetHash, StateUnknown))
		return fmt.Errorf("set host network %s ready (%v) failed [%w], error: %w configErr: %w", hnc.Name, l3setupErr == nil, categoryErr, statusUpdateErr, l3setupErr)
	}

	if l3setupErr != nil {
		h.stateMgr.CreateOrUpdate(hnc.Name, NewLocalHostNetworkConfigState(targetHash, StateUnknown))
		// Include categoryErr alongside l3setupErr for immediate visibility in controller logs
		return fmt.Errorf("setup host network config %s failed [%w]: configErr: %w", hnc.Name, categoryErr, l3setupErr)
	}

	// per node status is ready
	h.stateMgr.CreateOrUpdate(hnc.Name, NewLocalHostNetworkConfigState(targetHash, StateReady))
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

	// 1. Stop lease manager first to release sockets and terminate DHCP process on the sub-interface
	h.stopLeaseManager(utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID))

	// 2. Remove VLAN sub-interface
	if err := bridgelink.DelVlanSubInterface(hnc.Spec.VlanID); err != nil {
		return nil, fmt.Errorf("del vlan subinterface %d failed for %s, error: %w", hnc.Spec.VlanID, v.Bridge().Name, err)
	}

	// 3. Remove bridge VLAN entry
	if err := bridgelink.DelBridgeVlanSelf(hnc.Spec.VlanID); err != nil {
		return nil, fmt.Errorf("del bridge vlanconfig %d failed for %s, error: %w", hnc.Spec.VlanID, v.Bridge().Name, err)
	}

	// 4. Reconcile cluster network to delete vid from the uplink(cluster-bo)
	if err := h.wakeUpClusterNetwork(hnc.Spec.ClusterNetwork); err != nil {
		return nil, fmt.Errorf("wake up cluster network %s failed, error: %w", hnc.Spec.ClusterNetwork, err)
	}

	// 5. Update per-node status when interface deleted due to node selector changes.
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

	// Delete local state tracking entry when the CRD is being removed.
	h.stateMgr.Delete(hnc.Name)

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

	// JSON Merge Patch (RFC 7396): setting a map key to null removes it from the map
	patchPayload := map[string]interface{}{
		"status": map[string]interface{}{
			"nodeStatus": map[string]interface{}{
				h.nodeName: nil,
			},
		},
	}

	patchBytes, err := json.Marshal(patchPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal patch payload: %w", err)
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

// setHostNetworkPerNodeStatus updates the node status conditions for the HostNetworkConfig.
// We use high-level category errors (sentinel errors like ErrL2NotReady or ErrL3NotReady)
// rather than raw low-level syscall or netlink errors. CRDs are control-plane state signals,
// not log aggregators; operators checking a degraded CRD status get a clear high-level
// reason, while granular debugging details remain in the controller pod logs. This design
// also prevents etcd write inflation and watch churn from transient, dynamic error text.
func (h *Handler) setHostNetworkPerNodeStatus(hnc *networkv1.HostNetworkConfig, ready bool, setupErr error, categoryErr error) error {
	readyStatus := "True"
	message := ""
	if !ready {
		readyStatus = "False"
		message = fmt.Sprintf("setup l3 connectivity failed: %v", categoryErr)
	}

	patchPayload := map[string]interface{}{
		"status": map[string]interface{}{
			"nodeStatus": map[string]interface{}{
				h.nodeName: map[string]interface{}{
					"clusterNetwork": hnc.Spec.ClusterNetwork,
					"vlanID":         hnc.Spec.VlanID,
					"mode":           hnc.Spec.Mode,
					"conditions": []map[string]interface{}{
						{
							"type":    networkv1.Ready,
							"status":  readyStatus,
							"message": message,
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

func (h *Handler) setHostNetworkStatus(hnc *networkv1.HostNetworkConfig, setupErr error, categoryErr error) error {
	if setupErr != nil {
		return h.setHostNetworkPerNodeStatus(hnc, false, setupErr, categoryErr)
	}

	return h.setHostNetworkPerNodeStatus(hnc, true, nil, categoryErr)
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

func (h *Handler) handleNonMatchingNode(hnc *networkv1.HostNetworkConfig, targetHash string) (*networkv1.HostNetworkConfig, error) {
	if h.isAlreadyRemoved(hnc, targetHash) {
		logrus.Debugf("hostnetwork config %s is already removed on node %s, fast exit", hnc.Name, h.nodeName)
		return hnc, nil
	}

	// Teardown netlink interface and patch node status
	if _, err := h.removeHostNetworkInterface(hnc, true); err != nil {
		return nil, err
	}

	// Cache is updated to StateRemoved on success
	h.stateMgr.CreateOrUpdate(hnc.Name, NewLocalHostNetworkConfigState(targetHash, StateRemoved))

	return hnc, nil
}

func (h *Handler) isAlreadyRemoved(hnc *networkv1.HostNetworkConfig, targetHash string) bool {
	// 1. Check local state cache
	localState, exists := h.stateMgr.Get(hnc.Name)
	if !exists || !localState.IsRemoved(targetHash, h.stateMgr.TTL()) {
		return false
	}

	// 2. Check remote CRD status: nodeStatus for this node must be cleared
	if hnc == nil || hnc.Status.NodeStatus == nil {
		return true
	}

	_, statusExists := hnc.Status.NodeStatus[h.nodeName]
	return !statusExists
}

func (h *Handler) isAlreadyReady(hnc *networkv1.HostNetworkConfig, targetHash string) bool {
	// 1. Check local state cache
	localState, exists := h.stateMgr.Get(hnc.Name)
	if !exists || !localState.IsReady(targetHash, h.stateMgr.TTL()) {
		return false
	}

	// 2. Check remote CRD status
	if hnc == nil || hnc.Status.NodeStatus == nil {
		return false
	}

	nodeStatus, exists := hnc.Status.NodeStatus[h.nodeName]
	if !exists {
		return false
	}

	// 3. Verify conditions array contains type Ready with status "True"
	for _, cond := range nodeStatus.Conditions {
		if cond.Type == networkv1.Ready && cond.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}
