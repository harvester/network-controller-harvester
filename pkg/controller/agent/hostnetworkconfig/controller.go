package hostnetworkconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
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

	controllerEnqueueSelfInterval = 1000 // ms
)

type Handler struct {
	nodeName              string
	nodeClient            ctlcorev1.NodeClient
	nodeCache             ctlcorev1.NodeCache
	hostNetworkClient     ctlnetworkv1.HostNetworkConfigClient
	hostNetworkCache      ctlnetworkv1.HostNetworkConfigCache
	hostNetworkController ctlnetworkv1.HostNetworkConfigController
	cnCache               ctlnetworkv1.ClusterNetworkCache
	cnController          ctlnetworkv1.ClusterNetworkController

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
		nodeName:              management.Options.NodeName,
		nodeClient:            nodes,
		nodeCache:             nodes.Cache(),
		hostNetworkClient:     hns,
		hostNetworkCache:      hns.Cache(),
		hostNetworkController: hns,
		cnCache:               cns.Cache(),
		cnController:          cns,
		leaseManagers:         make(map[string]*LeaseManager),
	}

	if mgmtIntf, err = iface.GetMgmtInterface(); err != nil {
		return fmt.Errorf("failed to get management interface for node %s, error: %w", handler.nodeName, err)
	}
	handler.mgmtIntfName = mgmtIntf

	hns.OnChange(ctx, ControllerName, handler.OnChange)
	hns.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
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

	intfName := utils.GetClusterNetworkBrVlanDevice(utils.GenerateBridgeName(hnc.Spec.ClusterNetwork), hnc.Spec.VlanID)

	// node selector doesn't match, need to clean up the host network config if exists
	if !matchNodeSet {
		return h.cleanupStaleHostNetworkConfig(hnc)
	}

	// match this node, ensure everything from beginning
	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			logrus.Infof("cluster network %s is not set on this node, wait for it to be setup", hnc.Spec.ClusterNetwork)
			//stop and delete all lease manaagers assosciated with the cluster network (if uplink removed due to vlanconfig changes/deletion)
			h.stopLeaseManager(intfName)
		}
		// retry anyway
		return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
	}

	var bridgelink *iface.Link
	bridgelink, err = v.GetBridgelink()
	if err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
	}

	if err = bridgelink.AddBridgeVlanSelf(hnc.Spec.VlanID); err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
	}

	if err = bridgelink.CreateVlanSubInterface(hnc.Spec.VlanID); err != nil {
		return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
	}

	// reconcile cluster network to add vid to the uplink(cluster-bo)
	if err := h.wakeUpClusterNetwork(hnc.Spec.ClusterNetwork); err != nil {
		return nil, fmt.Errorf("wake up cluster network %s failed, error: %w", hnc.Spec.ClusterNetwork, err)
	}

	var nns *networkv1.NodeNetworkState
	var ips *networkv1.MultiFamilyIP
	switch hnc.Spec.Mode {
	case IPModeDHCP:
		nns, err = h.startLeaseManager(bridgelink, hnc)
		if err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
		} else {
			logrus.Infof("hostnetwork config %s on node %s gets ip %v from dhcp server", hnc.Name, h.nodeName, nns)
		}

	case IPModeStatic:
		// stop lease manager if exists (previously in dhcp mode)
		h.stopLeaseManager(intfName)

		ips, err = findMatchingIPfromNode(h.nodeName, hnc)
		if err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
		}

		// if controller gets ErrCleanupIP* error, set CRD status as ready, but log & retry locally
		err = bridgelink.SetMultiFamilyIPAddresses(iface.NewMultiFamilyIPWithLeaseFromStaticIPs(ips), hnc.Spec.VlanID)
		if err != nil {
			switch err {
			case iface.ErrCleanupIPv4, iface.ErrCleanupIPv6, iface.ErrCleanupIPv4v6:
				logrus.Warnf("Static IP configuration for interface %s succeeded, but stale address cleanup failed (will retry): %v", intfName, err)
				h.hostNetworkController.EnqueueAfter(hnc.Name, controllerEnqueueSelfInterval*time.Millisecond)
			default:
				return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
			}
		}
		nns = ConvertMultiFamilyIPToNodeNetworkState(ips)

	default:
		err = fmt.Errorf("unsupported ip assignment mode %s for host network config %s", hnc.Spec.Mode, hnc.Name)
		return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
	}

	// success case, update host network config status to ready
	if updateErr := h.updateHostNetworkReadyStatus(hnc, nil, nns); updateErr != nil {
		return hnc, updateErr
	}

	// if dhcp mode fails on last step, some stale IPs are not cleaned, then trigger self to retry
	if hnc.Spec.Mode == IPModeDHCP {
		if h.cleanupStaleIP(intfName) {
			h.hostNetworkController.EnqueueAfter(hnc.Name, controllerEnqueueSelfInterval*time.Millisecond)
		}
	}

	// update node annotation to set the vlan sub interface to be used as underlay (if underlay is enabled)
	// and set to default mgmt interface if underlay is not enabled
	if err := h.addNodeAnnotation(intfName, hnc.Spec.Underlay); err != nil {
		return nil, fmt.Errorf("add node annotation to node %s for host network config %s failed, error: %w", h.nodeName, hnc.Name, err)
	}

	return hnc, nil
}

func (h *Handler) updateHostNetworkReadyStatus(hnc *networkv1.HostNetworkConfig, l3setupErr error, nns *networkv1.NodeNetworkState) error {
	if statusUpdateErr := h.setHostNetworkStatus(hnc, l3setupErr, nns); statusUpdateErr != nil {
		return fmt.Errorf("set host network %s unready failed, error: %w configErr: %v", hnc.Name, statusUpdateErr, l3setupErr)
	}

	if l3setupErr != nil {
		return fmt.Errorf("setup host network config %s failed, error: %w", hnc.Name, l3setupErr)
	}

	return nil
}

func (h *Handler) removeHostNetworkInterface(hnc *networkv1.HostNetworkConfig) error {
	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			// stop anyway
			h.stopLeaseManager(utils.GetClusterNetworkBrVlanDevice(utils.GenerateBridgeName(hnc.Spec.ClusterNetwork), hnc.Spec.VlanID))
			return nil
		}
		return err
	}

	bridgelink, err := v.GetBridgelink()
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			h.stopLeaseManager(utils.GetClusterNetworkBrVlanDevice(utils.GenerateBridgeName(hnc.Spec.ClusterNetwork), hnc.Spec.VlanID))
			return nil
		} else {
			return fmt.Errorf("failed to get link for bridge %s, error: %w", v.Bridge().Name, err)
		}
	}

	// the normal cleanup path, release the dhcp lease before tearing down the vlan
	h.stopLeaseManager(utils.GetClusterNetworkBrVlanDevice(utils.GenerateBridgeName(hnc.Spec.ClusterNetwork), hnc.Spec.VlanID))

	if err := bridgelink.DelBridgeVlanSelf(hnc.Spec.VlanID); err != nil {
		return fmt.Errorf("del bridge vlanconfig %d failed for %s, error: %w", hnc.Spec.VlanID, v.Bridge().Name, err)
	}

	if err := bridgelink.DelVlanSubInterface(hnc.Spec.VlanID); err != nil {
		return fmt.Errorf("del vlan subinterface %d failed for %s, error: %w", hnc.Spec.VlanID, v.Bridge().Name, err)
	}

	// reconcile cluster network to delete vid from the uplink(cluster-bo)
	if err := h.wakeUpClusterNetwork(hnc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf("wake up cluster network %s failed, error: %w", hnc.Spec.ClusterNetwork, err)
	}

	return nil
}

func (h *Handler) OnRemove(_ string, hnc *networkv1.HostNetworkConfig) (*networkv1.HostNetworkConfig, error) {
	if hnc == nil {
		return nil, nil
	}

	logrus.Infof("hostnetwork config %s has been removed, spec: %+v", hnc.Name, hnc.Spec)
	// OnRemove just clears up local object, but does not update CRD status due to it is vanishing
	if err := h.removeHostNetworkInterface(hnc); err != nil {
		return nil, err
	}
	return hnc, nil
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

func findMatchingIPfromNode(nodeName string, hnc *networkv1.HostNetworkConfig) (*networkv1.MultiFamilyIP, error) {
	// legacy ipv4 mode
	if len(hnc.Spec.HostIPs) != 0 {
		ip := hnc.Spec.HostIPs[nodeName]
		if ip == "" {
			return nil, fmt.Errorf("no matching IP found for node %s", nodeName)
		}
		return &networkv1.MultiFamilyIP{IPv4: &ip}, nil
	}

	// multi-family IPs
	ips, ok := hnc.Spec.HostMultiFamilyIPs[nodeName]
	if ok {
		return &ips, nil
	}

	// if no matching IP found for the node, return error to set host network config status to not ready
	return nil, fmt.Errorf("no matching multi-family IPs found for node %s", nodeName)
}

func (h *Handler) removeHostNetworkPerNodeStatus(hnc *networkv1.HostNetworkConfig) (*networkv1.HostNetworkConfig, error) {
	if hnc.Status.NodeStatus == nil {
		return hnc, nil
	}

	if _, exists := hnc.Status.NodeStatus[h.nodeName]; !exists {
		return hnc, nil
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
		return nil, fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = h.hostNetworkClient.Patch(
		hnc.Name,
		types.MergePatchType,
		patchBytes,
		"status",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to patch HostNetworkConfig status for node %s: %w", h.nodeName, err)
	}

	return hnc, nil
}

// setHostNetworkPerNodeStatus updates the node-specific status of the HostNetworkConfig.
// Design Note: In error/failure cases (ready = false), the "network" field is intentionally omitted
// from the patch payload. This preserves the last-known-good active network configuration on the server
// for debugging, allowing operators to easily compare the desired spec against the old working state.
func (h *Handler) setHostNetworkPerNodeStatus(hnc *networkv1.HostNetworkConfig, ready bool, setupErr error, nns *networkv1.NodeNetworkState) error {
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}

	message := ""
	if !ready {
		message = fmt.Sprintf("setup l3 connectivity failed: %v", setupErr)
	}

	// 1. Retrieve the existing network state for this node if it already exists.
	// This allows us to preserve it during failure scenarios and build an accurate
	// desired state representation for the DeepEqual comparison.
	var existingNetwork *networkv1.NodeNetworkState
	if hnc.Status.NodeStatus != nil {
		if existingNodeStatus, exists := hnc.Status.NodeStatus[h.nodeName]; exists {
			existingNetwork = existingNodeStatus.Network
		}
	}

	// 2. Determine the target network state:
	// - If ready is true and a new nns is provided, use the new nns.
	// - If ready is false (failure case), fall back to preserving the existing network
	//   state for debugging visibility.
	targetNetwork := existingNetwork
	if ready && nns != nil {
		targetNetwork = nns
	}

	// 3. Construct the desired node status struct for equality checking
	desiredNodeStatus := networkv1.HostNetworkConfigNodeStatus{
		Mode:     hnc.Spec.Mode,
		IPFamily: hnc.Spec.IPFamily,
		Network:  targetNetwork,
		Conditions: []networkv1.Condition{
			{
				Type:    networkv1.Ready,
				Status:  status,
				Message: message,
			},
		},
	}

	// 4. Perform DeepEqual check to avoid redundant API server updates
	if hnc.Status.NodeStatus != nil {
		if existing, exists := hnc.Status.NodeStatus[h.nodeName]; exists {
			if equality.Semantic.DeepEqual(existing, desiredNodeStatus) {
				// Status is completely identical; skip patch call
				return nil
			}
		}
	}

	// 5. Build the patch payload
	nodeStatusMap := map[string]interface{}{
		"mode":     hnc.Spec.Mode,
		"ipFamily": hnc.Spec.IPFamily,
		"conditions": []map[string]interface{}{
			{
				"type":    networkv1.Ready,
				"status":  string(status),
				"message": message,
			},
		},
	}

	// Only include the "network" field in the patch when ready is true and nns is present.
	// Omitting it on failure ensures JSON Merge Patch leaves the server's existing
	// network field untouched.
	if ready && nns != nil {
		nodeStatusMap["network"] = map[string]interface{}{
			"ipv4":           nns.IPv4,
			"ipv6":           nns.IPv6,
			"gatewayIPv4":    nns.GatewayIPv4,
			"dhcpServerIPv4": nns.DHCPServerIPv4,
		}
	}

	patchPayload := map[string]interface{}{
		"status": map[string]interface{}{
			"nodeStatus": map[string]interface{}{
				h.nodeName: nodeStatusMap,
			},
		},
	}

	patchBytes, err := json.Marshal(patchPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	// 6. Submit the patch to the API server
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

func (h *Handler) setHostNetworkStatus(hnc *networkv1.HostNetworkConfig, setupErr error, nns *networkv1.NodeNetworkState) error {
	if setupErr != nil {
		return h.setHostNetworkPerNodeStatus(hnc, false, setupErr, nns)
	} else {
		return h.setHostNetworkPerNodeStatus(hnc, true, nil, nns)
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
	lm.StopLease()
}

func (h *Handler) startLeaseManager(bridgelink *iface.Link, hnc *networkv1.HostNetworkConfig) (*networkv1.NodeNetworkState, error) {
	vlanIntfName := utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID)

	h.mu.Lock()

	if lm, exists := h.leaseManagers[vlanIntfName]; exists && lm != nil {
		// If the HNC UID changed, the previous object was deleted and recreated quickly.
		// Stop the stale LeaseManager and replace it.
		if lm.hncUID != hnc.UID {
			delete(h.leaseManagers, vlanIntfName)
			h.mu.Unlock()

			logrus.Infof("HNC UID change detected for interface %s (old UID: %s, new UID: %s). Replacing stale LeaseManager...",
				vlanIntfName, lm.hncUID, hnc.UID)
			lm.StopLease() // Cancel background renewal loops and cleanup contexts outside the lock

		} else {
			h.mu.Unlock()
			// Existing LeaseManager is up-to-date; return its current state
			state := lm.GetNetworkState()
			return &state, nil
		}
	} else {
		// If it didn't exist in the map, release the initial lock before creation
		h.mu.Unlock()
	}

	// Create and start a new LeaseManager for this HNC incarnation (done outside the lock)
	lm, state, err := NewLeaseManager(vlanIntfName, bridgelink, hnc, h.hostNetworkController)
	if err != nil {
		return nil, err
	}

	h.mu.Lock()
	h.leaseManagers[vlanIntfName] = lm
	h.mu.Unlock()

	return state, nil
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

	// set to default mgmt interface if underlay is not enabled
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

// ConvertMultiFamilyIPToNodeNetworkState converts a MultiFamilyIP configuration
// into a NodeNetworkState containing the string representations of the IP addresses.
func ConvertMultiFamilyIPToNodeNetworkState(mf *networkv1.MultiFamilyIP) *networkv1.NodeNetworkState {
	nns := networkv1.NodeNetworkState{}
	if mf == nil {
		return nil
	}

	if mf.IPv4 != nil {
		// Note: adjust `.String()` to match your IPAddr type's string method or field
		// (e.g., mf.IPv4.IP.String() or mf.IPv4.String())
		nns.IPv4 = mf.IPv4.String()
	}

	if mf.IPv6 != nil {
		nns.IPv6 = mf.IPv6.String()
	}

	return &nns
}

func (h *Handler) cleanupStaleIP(vlanIntfName string) bool {
	h.mu.Lock()
	lm := h.leaseManagers[vlanIntfName]
	h.mu.Unlock()
	if lm == nil {
		return false
	}

	return lm.EnsureCleanup()
}

// cleanup potential stale hostnetworkconfig
func (h *Handler) cleanupStaleHostNetworkConfig(hnc *networkv1.HostNetworkConfig) (*networkv1.HostNetworkConfig, error) {
	if err := h.removeHostNetworkInterface(hnc); err != nil {
		return nil, err
	}
	// always ensure the status is cleaned
	return h.removeHostNetworkPerNodeStatus(hnc)
}
