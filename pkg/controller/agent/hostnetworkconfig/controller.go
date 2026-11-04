package hostnetworkconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

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

func checkifHostNetworkInterfaceExists(hnc *networkv1.HostNetworkConfig) (exists bool, isUp bool, err error) {
	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			return false, false, nil
		}
		return false, false, err
	}

	bridgelink, err := v.GetBridgelink()
	if err != nil {
		return false, false, err
	}

	vlanIntf := utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID)
	vlanlink, err := netlink.LinkByName(vlanIntf)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			return false, false, nil
		}
		return false, false, err
	}

	// Check if the interface has the UP flag set
	if vlanlink.Attrs().Flags&net.FlagUp == 0 {
		return true, false, nil
	}

	return true, true, nil
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

	intfExists, intfUp, err := checkifHostNetworkInterfaceExists(hnc)
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

	// For DHCP mode: Skip processing only if the interface exists, up AND the LeaseManager is running.
	// For Static mode: Do NOT shortcut. Interface presence does not guarantee that IP/routes are applied
	// or that spec changes (e.g., updating to a new IP) are handled. `findMatchingIPfromNode` could also fail.
	//
	// Mode transitions is also secured by the reduced shortcut scope:
	// - DHCP to Static: Bypasses shortcut to ensure full reconciliation stops the LeaseManager.
	// - Static to DHCP: Won't trigger shortcut because LeaseManager isn't running yet, ensuring
	//   the new DHCP process starts properly.
	if intfExists && intfUp && (hnc.Spec.Mode == IPModeDHCP && h.isLeaseManagerRunning(intfName)) {
		logrus.Infof("hostnetwork config %s has been applied on this node already, update nodestatus,tunnel interface annotation and skip", hnc.Name)

		// intf exists but there could be change in underlay, need to update node annotation with new interface if needed
		if err := h.addNodeAnnotation(intfName, hnc.Spec.Underlay); err != nil {
			return nil, fmt.Errorf("add node annotation to node %s for host network config %s failed, error: %w", h.nodeName, hnc.Name, err)
		}

		err := h.setHostNetworkPerNodeStatus(hnc, true, nil, nil)
		if err != nil {
			return nil, err
		}
		return hnc, nil
	}

	var bridgelink *iface.Link

	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			logrus.Infof("cluster network %s is not set on this node, skip", hnc.Spec.ClusterNetwork)
			//stop and delete all lease manaagers assosciated with the cluster network (if uplink removed due to vlanconfig changes/deletion)
			h.stopLeaseManager(intfName)
			return nil, nil
		}
		return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
	}

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
	switch hnc.Spec.Mode {
	case IPModeDHCP:
		nns, err = h.startLeaseManager(bridgelink, hnc.Spec.VlanID, hnc.Spec.IPFamily)
		if err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
		} else {
			logrus.Infof("hostnetwork config %s on node %s gets ip %v from dhcp server", hnc.Name, h.nodeName, nns)
		}

	case IPModeStatic:
		// stop lease manager if exists (previously in dhcp mode)
		h.stopLeaseManager(intfName)

		addr, err := findMatchingIPfromNode(h.nodeName, hnc)
		if err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
		}

		if err := bridgelink.SetMultiFamilyIPAddresses(addr, hnc.Spec.VlanID); err != nil {
			return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
		}
		nns = ConvertMultiFamilyIPToNodeNetworkState(addr)
	default:
		err = fmt.Errorf("unsupported ip assignment mode %s for host network config %s", hnc.Spec.Mode, hnc.Name)
		return hnc, h.updateHostNetworkReadyStatus(hnc, err, nil)
	}

	//success case, update host network config status to ready
	if updateErr := h.updateHostNetworkReadyStatus(hnc, nil, nns); updateErr != nil {
		return hnc, updateErr
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

func (h *Handler) removeHostNetworkInterface(hnc *networkv1.HostNetworkConfig, onChange bool) (*networkv1.HostNetworkConfig, error) {
	v, err := vlan.GetVlan(hnc.Spec.ClusterNetwork)
	if err != nil {
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			logrus.Debugf("cluster network %s is not set on this node, skip", hnc.Spec.ClusterNetwork)
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

	// Release the dhcp lease before tearing down the vlan
	h.stopLeaseManager(utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, hnc.Spec.VlanID))

	if err := bridgelink.DelBridgeVlanSelf(hnc.Spec.VlanID); err != nil {
		return nil, fmt.Errorf("del bridge vlanconfig %d failed for %s, error: %w", hnc.Spec.VlanID, v.Bridge().Name, err)
	}

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
	lm.Stop()
}

func (h *Handler) startLeaseManager(bridgelink *iface.Link, vlanID uint16, family networkv1.IPFamily) (ip *networkv1.NodeNetworkState, err error) {
	vlanIntfName := utils.GetClusterNetworkBrVlanDevice(bridgelink.Attrs().Name, vlanID)

	h.mu.Lock()
	lm := h.leaseManagers[vlanIntfName]
	h.mu.Unlock()

	created := false
	if lm == nil {
		lm, err = NewLeaseManager(vlanIntfName, bridgelink, vlanID, family)
		if err != nil {
			return nil, err
		}
		created = true
	}

	// if failed in middle way, Start will call Stop automatically
	state, err := lm.Start(context.Background())
	if err != nil {
		return nil, err
	}

	// fully ready, add to map
	if created {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.leaseManagers[vlanIntfName] = lm
	}

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

func (h *Handler) isLeaseManagerRunning(vlanIntfName string) bool {
	h.mu.Lock()
	lm := h.leaseManagers[vlanIntfName]
	h.mu.Unlock()

	if lm == nil {
		return false
	}

	return lm.IsRunning()
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
