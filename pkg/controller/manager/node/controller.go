package node

import (
	"context"
	"fmt"

	mapset "github.com/deckarep/golang-set/v2"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io"
	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const controllerName = "harvester-network-manager-node-controller"

type Handler struct {
	nodeClient ctlcorev1.NodeClient
	vcCache    ctlnetworkv1.VlanConfigCache
	vcClient   ctlnetworkv1.VlanConfigClient
	vsCache    ctlnetworkv1.VlanStatusCache
	vsClient   ctlnetworkv1.VlanStatusClient
	lmCache    ctlnetworkv1.LinkMonitorCache
	lmClient   ctlnetworkv1.LinkMonitorClient
}

func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	vcs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()
	vss := management.HarvesterNetworkFactory.Network().V1beta1().VlanStatus()
	lms := management.HarvesterNetworkFactory.Network().V1beta1().LinkMonitor()

	h := Handler{
		nodeClient: nodes,
		vcCache:    vcs.Cache(),
		vcClient:   vcs,
		vsCache:    vss.Cache(),
		vsClient:   vss,
		lmCache:    lms.Cache(),
		lmClient:   lms,
	}

	nodes.OnChange(ctx, controllerName, h.OnChange)
	nodes.OnRemove(ctx, controllerName, h.OnRemove)

	return nil
}

func (h Handler) OnChange(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return nil, nil
	}

	if err := h.ensureMgmtLabels(node); err != nil {
		return nil, err
	}

	// skip witness node because we do not allow vlan config on witness node
	if node.Labels != nil && node.Labels[utils.HarvesterWitnessNodeLabelKey] == "true" {
		return nil, nil
	}

	vcs, err := h.vcCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, vc := range vcs {
		if err := h.updateMatchedNodeAnnotation(vc, node); err != nil {
			return nil, fmt.Errorf("failed to update matched node annotation, vc: %s, node: %s, error: %w",
				vc.Name, node.Name, err)
		}
	}

	return node, nil
}

func (h Handler) OnRemove(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil {
		return nil, nil
	}

	klog.Infof("node %s is removed", node.Name)

	if err := h.removeNodeFromVlanConfig(node.Name); err != nil {
		return nil, err
	}
	if err := h.clearLinkStatus(node.Name); err != nil {
		return nil, err
	}

	return node, nil
}

func (h Handler) updateMatchedNodeAnnotation(vc *networkv1.VlanConfig, node *corev1.Node) error {
	selector, err := utils.NewSelector(vc.Spec.NodeSelector)
	if err != nil {
		return err
	}

	s := mapset.NewSet[string]()
	if vc.Annotations != nil && vc.Annotations[utils.KeyMatchedNodes] != "" {
		if err := s.UnmarshalJSON([]byte(vc.Annotations[utils.KeyMatchedNodes])); err != nil {
			return err
		}
	}

	newSet := s.Clone()
	if ok := selector.Matches(labels.Set(node.Labels)); ok {
		newSet.Add(node.Name)
	} else {
		newSet.Remove(node.Name)
	}

	if newSet.Equal(s) {
		return nil
	}

	vcCopy := vc.DeepCopy()
	if vcCopy.Annotations == nil {
		vcCopy.Annotations = map[string]string{}
	}
	bytes, err := newSet.MarshalJSON()
	if err != nil {
		return err
	}
	vcCopy.Annotations[utils.KeyMatchedNodes] = string(bytes)
	if _, err := h.vcClient.Update(vcCopy); err != nil {
		return err
	}

	return nil
}

func (h Handler) ensureMgmtLabels(node *corev1.Node) error {
	key := network.GroupName + "/" + utils.ManagementClusterNetworkName
	if node.Labels != nil && node.Labels[key] == utils.ValueTrue {
		return nil
	}

	nodeCopy := node.DeepCopy()
	if nodeCopy.Labels == nil {
		nodeCopy.Labels = make(map[string]string)
	}
	nodeCopy.Labels[key] = utils.ValueTrue
	if _, err := h.nodeClient.Update(nodeCopy); err != nil {
		return fmt.Errorf("update node %s failed, error: %w", node.Name, err)
	}

	return nil
}

// Clear link statuses related to the removed node
func (h Handler) clearLinkStatus(nodeName string) error {
	lms, err := h.lmCache.List(labels.Everything())
	if err != nil {
		return err
	}

	for _, lm := range lms {
		if _, ok := lm.Status.LinkStatus[nodeName]; ok {
			lmCopy := lm.DeepCopy()
			delete(lmCopy.Status.LinkStatus, nodeName)
			if _, err := h.lmClient.Update(lmCopy); err != nil {
				return fmt.Errorf("update link monitor status failed, lm: %s, node: %s, error: %w", lm.Name, nodeName, err)
			}
		}
	}

	return nil
}

// remove the node from the matched node list of the vlan config and the related vlan status
func (h Handler) removeNodeFromVlanConfig(nodeName string) error {
	vcs, err := h.vcCache.List(labels.Everything())
	if err != nil {
		return err
	}

	for _, vc := range vcs {
		if err := h.removeNodeFromOneVlanConfig(vc, nodeName); err != nil {
			return err
		}
	}

	return nil
}

func (h Handler) removeNodeFromOneVlanConfig(vc *networkv1.VlanConfig, nodeName string) error {
	if vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" {
		return nil
	}

	s := mapset.NewSet[string]()
	if err := s.UnmarshalJSON([]byte(vc.Annotations[utils.KeyMatchedNodes])); err != nil {
		return err
	}

	if s.Contains(nodeName) {
		s.Remove(nodeName)
		bytes, err := s.MarshalJSON()
		if err != nil {
			return err
		}
		vcCopy := vc.DeepCopy()
		vcCopy.Annotations[utils.KeyMatchedNodes] = string(bytes)
		if _, err := h.vcClient.Update(vcCopy); err != nil {
			return err
		}

		vss, err := h.vsCache.List(labels.Set{
			utils.KeyVlanConfigLabel: vc.Name,
			utils.KeyNodeLabel:       nodeName,
		}.AsSelector())
		if err != nil {
			return err
		}
		if len(vss) == 0 {
			return nil
		}
		if err := h.vsClient.Delete(vss[0].Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}
