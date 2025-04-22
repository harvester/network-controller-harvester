package vlanconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/vishvananda/netlink"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	"github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io"
	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/nad"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/network/vlan"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	ControllerName = "harvester-network-vlanconfig-controller"
	bridgeCNIName  = "bridge"
)

type Handler struct {
	nodeName   string
	nodeClient ctlcorev1.NodeClient
	nodeCache  ctlcorev1.NodeCache
	nadCache   ctlcniv1.NetworkAttachmentDefinitionCache
	vcClient   ctlnetworkv1.VlanConfigClient
	vcCache    ctlnetworkv1.VlanConfigCache
	vsClient   ctlnetworkv1.VlanStatusClient
	vsCache    ctlnetworkv1.VlanStatusCache
	cnClient   ctlnetworkv1.ClusterNetworkClient
	cnCache    ctlnetworkv1.ClusterNetworkCache
}

func Register(ctx context.Context, management *config.Management) error {
	vcs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()
	vss := management.HarvesterNetworkFactory.Network().V1beta1().VlanStatus()
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()
	nodes := management.CoreFactory.Core().V1().Node()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()

	handler := &Handler{
		nodeName:   management.Options.NodeName,
		nodeClient: nodes,
		nodeCache:  nodes.Cache(),
		nadCache:   nads.Cache(),
		vcClient:   vcs,
		vcCache:    vcs.Cache(),
		vsClient:   vss,
		vsCache:    vss.Cache(),
		cnClient:   cns,
		cnCache:    cns.Cache(),
	}

	if err := handler.initialize(); err != nil {
		return fmt.Errorf("initialize error: %w", err)
	}

	vcs.OnChange(ctx, ControllerName, handler.OnChange)
	vcs.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

func (h Handler) OnChange(_ string, vc *networkv1.VlanConfig) (*networkv1.VlanConfig, error) {
	if vc == nil || vc.DeletionTimestamp != nil {
		return nil, nil
	}
	klog.Infof("vlan config %s has been changed, spec: %+v", vc.Name, vc.Spec)

	isMatched, err := h.MatchNode(vc)
	if err != nil {
		return nil, err
	}

	vs, err := h.getVlanStatus(vc)
	if err != nil {
		return nil, err
	}

	if (!isMatched && vs != nil) || (isMatched && vs != nil && !matchClusterNetwork(vc, vs)) {
		if err := h.removeVLAN(vs); err != nil {
			return nil, err
		}
	}

	// set up VLAN
	if isMatched {
		if err := h.setupVLAN(vc); err != nil {
			return nil, err
		}
	}

	return vc, nil
}

func (h Handler) OnRemove(_ string, vc *networkv1.VlanConfig) (*networkv1.VlanConfig, error) {
	if vc == nil {
		return nil, nil
	}

	klog.Infof("vlan config %s has been removed", vc.Name)

	vs, err := h.getVlanStatus(vc)
	if err != nil {
		return nil, err
	}

	if vs != nil {
		if err := h.removeVLAN(vs); err != nil {
			return nil, err
		}
	}

	return vc, nil
}

func (h Handler) initialize() error {
	if err := iface.DisableBridgeNF(); err != nil {
		return fmt.Errorf("disable net.bridge.bridge-nf-call-iptables failed, error: %v", err)
	}
	return nil
}

// MatchNode will also return the executed vlanconfig with the same clusterNetwork on this node if existing
func (h Handler) MatchNode(vc *networkv1.VlanConfig) (bool, error) {
	if vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" {
		return false, nil
	}

	var matchedNodes []string
	if err := json.Unmarshal([]byte(vc.Annotations[utils.KeyMatchedNodes]), &matchedNodes); err != nil {
		return false, err
	}

	klog.Infof("matchedNodes: %+v, h.nodeName: %s", matchedNodes, h.nodeName)

	for _, n := range matchedNodes {
		if n == h.nodeName {
			return true, nil
		}
	}

	return false, nil
}

func (h Handler) getVlanStatus(vc *networkv1.VlanConfig) (*networkv1.VlanStatus, error) {
	vss, err := h.vsCache.List(labels.Set(map[string]string{
		utils.KeyVlanConfigLabel: vc.Name,
		utils.KeyNodeLabel:       h.nodeName,
	}).AsSelector())
	if err != nil {
		return nil, err
	}

	switch len(vss) {
	case 0:
		// We take it granted that the empty vlanstatus matches the vlanconfig
		return nil, nil
	case 1:
		return vss[0], nil
	default:
		return nil, fmt.Errorf("invalid vlanstatus list for vlanconfig %s on node %s", vc.Name, h.nodeName)
	}
}

func matchClusterNetwork(vc *networkv1.VlanConfig, vs *networkv1.VlanStatus) bool {
	return vc.Spec.ClusterNetwork == vs.Status.ClusterNetwork
}

func (h Handler) setupVLAN(vc *networkv1.VlanConfig) error {
	var v *vlan.Vlan
	var setupErr error
	var localAreas []*vlan.LocalArea
	var uplink *iface.Link
	// get VIDs
	localAreas, setupErr = h.getLocalAreas(vc)
	if setupErr != nil {
		goto updateStatus
	}
	// construct uplink
	uplink, setupErr = setUplink(vc)
	if setupErr != nil {
		goto updateStatus
	}
	// set up VLAN
	v = vlan.NewVlan(vc.Spec.ClusterNetwork)
	if setupErr = v.Setup(uplink); setupErr != nil {
		goto updateStatus
	}
	setupErr = h.AddLocalAreas(v, localAreas)

updateStatus:
	// Update status and still return setup error if not nil
	if err := h.updateStatus(vc, localAreas, setupErr); err != nil {
		return fmt.Errorf("update status into vlanstatus %s failed, error: %w, setup error: %v",
			h.statusName(vc.Spec.ClusterNetwork), err, setupErr)
	}
	if setupErr != nil {
		return fmt.Errorf("set up VLAN failed, vlanconfig: %s, node: %s, error: %w", vc.Name, h.nodeName, setupErr)
	}
	// update node labels for pod scheduling
	if err := h.addNodeLabel(vc); err != nil {
		return fmt.Errorf("add node label to node %s for vlanconfig %s failed, error: %w", h.nodeName, vc.Name, err)
	}

	return nil
}

func (h Handler) removeVLAN(vs *networkv1.VlanStatus) error {
	var v *vlan.Vlan
	var teardownErr error

	v, teardownErr = vlan.GetVlan(vs.Status.ClusterNetwork)
	// We take it granted that `LinkNotFound` means the VLAN has been torn down.
	if teardownErr != nil {
		// ignore the LinkNotFound error
		if errors.As(teardownErr, &netlink.LinkNotFoundError{}) {
			teardownErr = nil
		}
		goto updateStatus
	}
	if teardownErr = v.Teardown(); teardownErr != nil {
		goto updateStatus
	}

updateStatus:
	if err := h.removeNodeLabel(vs); err != nil {
		return err
	}
	if err := h.deleteStatus(vs, teardownErr); err != nil {
		return fmt.Errorf("update status into vlanstatus %s failed, error: %w, teardown error: %v",
			h.statusName(vs.Status.ClusterNetwork), err, teardownErr)
	}
	if teardownErr != nil {
		return fmt.Errorf("tear down VLAN failed, vlanconfig: %s, node: %s, error: %w", vs.Status.VlanConfig, h.nodeName, teardownErr)
	}

	return nil
}

func setUplink(vc *networkv1.VlanConfig) (*iface.Link, error) {
	// set link attributes
	linkAttrs := netlink.NewLinkAttrs()
	linkAttrs.Name = vc.Spec.ClusterNetwork + iface.BondSuffix
	if vc.Spec.Uplink.LinkAttrs != nil {
		linkAttrs.MTU = vc.Spec.Uplink.LinkAttrs.MTU
		linkAttrs.TxQLen = vc.Spec.Uplink.LinkAttrs.TxQLen
		if vc.Spec.Uplink.LinkAttrs.HardwareAddr != nil {
			linkAttrs.HardwareAddr = vc.Spec.Uplink.LinkAttrs.HardwareAddr
		}
	}
	// Note: do not use &netlink.Bond{}
	bond := netlink.NewLinkBond(linkAttrs)
	// set bonding mode
	mode := netlink.BOND_MODE_ACTIVE_BACKUP
	if vc.Spec.Uplink.BondOptions != nil && vc.Spec.Uplink.BondOptions.Mode != "" {
		mode = netlink.StringToBondMode(string(vc.Spec.Uplink.BondOptions.Mode))
	}
	bond.Mode = mode
	// set bonding miimon
	if vc.Spec.Uplink.BondOptions != nil {
		bond.Miimon = vc.Spec.Uplink.BondOptions.Miimon
	}

	b := iface.NewBond(bond, vc.Spec.Uplink.NICs)
	if err := b.EnsureBond(); err != nil {
		return nil, err
	}

	return &iface.Link{Link: b}, nil
}

func (h Handler) getLocalAreas(vc *networkv1.VlanConfig) ([]*vlan.LocalArea, error) {
	nads, err := h.nadCache.List("", labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: vc.Spec.ClusterNetwork,
	}).AsSelector())
	if err != nil {
		return nil, fmt.Errorf("list nad failed, error: %v", err)
	}

	localAreas := make([]*vlan.LocalArea, 0)
	for _, n := range nads {
		if !utils.IsVlanNad(n) {
			continue
		}
		localArea, err := nad.GetLocalArea(n.Labels[utils.KeyVlanLabel], n.Annotations[utils.KeyNetworkRoute])
		if err != nil {
			return nil, fmt.Errorf("failed to get local area from nad %s/%s, error: %w", n.Namespace, n.Name, err)
		}
		localAreas = append(localAreas, localArea)
	}

	return localAreas, nil
}

func (h Handler) AddLocalAreas(v *vlan.Vlan, localAreas []*vlan.LocalArea) error {
	for _, la := range localAreas {
		if err := v.AddLocalArea(la); err != nil {
			return fmt.Errorf("add local area %v failed, error: %w", la, err)
		}
	}

	return nil
}

func (h Handler) updateStatus(vc *networkv1.VlanConfig, localAreas []*vlan.LocalArea, setupErr error) error {
	var vStatus *networkv1.VlanStatus
	name := h.statusName(vc.Spec.ClusterNetwork)
	vs, getErr := h.vsCache.Get(name)
	if getErr != nil && !apierrors.IsNotFound(getErr) {
		return fmt.Errorf("could not get vlanstatus %s, error: %w", name, getErr)
	} else if apierrors.IsNotFound(getErr) {
		vStatus = &networkv1.VlanStatus{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					utils.KeyVlanConfigLabel:     vc.Name,
					utils.KeyClusterNetworkLabel: vc.Spec.ClusterNetwork,
					utils.KeyNodeLabel:           h.nodeName,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: vc.APIVersion,
						Kind:       vc.Kind,
						Name:       vc.Name,
						UID:        vc.UID,
					},
				},
			},
		}
	} else {
		vStatus = vs.DeepCopy()
	}

	vStatus.Labels = map[string]string{
		utils.KeyClusterNetworkLabel: vc.Spec.ClusterNetwork,
		utils.KeyVlanConfigLabel:     vc.Name,
		utils.KeyNodeLabel:           h.nodeName,
	}
	vStatus.Status.ClusterNetwork = vc.Spec.ClusterNetwork
	vStatus.Status.VlanConfig = vc.Name
	vStatus.Status.LinkMonitor = vc.Spec.ClusterNetwork
	vStatus.Status.Node = h.nodeName
	if setupErr == nil {
		networkv1.Ready.SetStatusBool(vStatus, true)
		networkv1.Ready.Message(vStatus, "")
		vStatus.Status.LocalAreas = []networkv1.LocalArea{}
		for _, la := range localAreas {
			vStatus.Status.LocalAreas = append(vStatus.Status.LocalAreas, networkv1.LocalArea{
				VID:  la.Vid,
				CIDR: la.Cidr,
			})
		}
	} else {
		networkv1.Ready.SetStatusBool(vStatus, false)
		networkv1.Ready.Message(vStatus, setupErr.Error())
	}

	if getErr != nil {
		if _, err := h.vsClient.Create(vStatus); err != nil {
			return fmt.Errorf("failed to create vlanstatus %s, error: %w", name, err)
		}
	} else {
		if _, err := h.vsClient.Update(vStatus); err != nil {
			return fmt.Errorf("failed to update vlanstatus %s, error: %w", name, err)
		}
	}

	return nil
}

func (h Handler) deleteStatus(vs *networkv1.VlanStatus, teardownErr error) error {
	if teardownErr != nil {
		vsCopy := vs.DeepCopy()
		networkv1.Ready.SetStatusBool(vsCopy, false)
		networkv1.Ready.Message(vsCopy, teardownErr.Error())
		if _, err := h.vsClient.Update(vsCopy); err != nil {
			return fmt.Errorf("failed to update vlanstatus %s, error: %w", vs.Name, err)
		}
	} else {
		if err := h.vsClient.Delete(vs.Name, &metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete vlanstatus %s, error: %w", vs.Name, err)
		}
	}

	return nil
}

func (h Handler) addNodeLabel(vc *networkv1.VlanConfig) error {
	node, err := h.nodeCache.Get(h.nodeName)
	if err != nil {
		return err
	}
	// Since the length of cluster network isn't bigger than 12, the length of key will less than 63.
	key := labelKeyOfClusterNetwork(vc.Spec.ClusterNetwork)
	if node.Labels != nil && node.Labels[key] == utils.ValueTrue &&
		node.Labels[utils.KeyVlanConfigLabel] == vc.Name {
		return nil
	}

	nodeCopy := node.DeepCopy()
	if nodeCopy.Labels == nil {
		nodeCopy.Labels = make(map[string]string)
	}
	nodeCopy.Labels[key] = utils.ValueTrue
	nodeCopy.Labels[utils.KeyVlanConfigLabel] = vc.Name

	if _, err := h.nodeClient.Update(nodeCopy); err != nil {
		return fmt.Errorf("add labels for vlanconfig %s to node %s failed, error: %w", vc.Name, h.nodeName, err)
	}

	return nil
}

func (h Handler) removeNodeLabel(vs *networkv1.VlanStatus) error {
	node, err := h.nodeCache.Get(h.nodeName)
	if err != nil {
		return err
	}

	key := labelKeyOfClusterNetwork(vs.Status.ClusterNetwork)
	if node.Labels != nil && (node.Labels[key] == utils.ValueTrue ||
		node.Labels[utils.KeyVlanConfigLabel] == vs.Status.VlanConfig) {
		nodeCopy := node.DeepCopy()
		delete(nodeCopy.Labels, key)
		delete(nodeCopy.Labels, utils.KeyVlanConfigLabel)
		if _, err := h.nodeClient.Update(nodeCopy); err != nil {
			return fmt.Errorf("remove labels for vlanconfig %s from node %s failed, error: %w", vs.Status.VlanConfig, h.nodeName, err)
		}
	}

	return nil
}

// vlanstatus name: <vc name>-<node name>-<crc32 checksum>
func (h Handler) statusName(clusterNetwork string) string {
	return utils.Name("", clusterNetwork, h.nodeName)
}

func labelKeyOfClusterNetwork(clusterNetwork string) string {
	return network.GroupName + "/" + clusterNetwork
}
