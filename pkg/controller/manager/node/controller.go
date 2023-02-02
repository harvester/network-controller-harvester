package node

import (
	"context"
	"fmt"

	mapset "github.com/deckarep/golang-set/v2"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

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
}

func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	vcs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()

	h := Handler{
		nodeClient: nodes,
		vcCache:    vcs.Cache(),
		vcClient:   vcs,
	}

	nodes.OnChange(ctx, controllerName, h.OnChange)

	return nil
}

func (h Handler) OnChange(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return nil, nil
	}

	if err := h.ensureMgmtLabels(node); err != nil {
		return nil, err
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

func (h Handler) updateMatchedNodeAnnotation(vc *networkv1.VlanConfig, node *corev1.Node) error {
	selector, err := utils.NewSelector(vc.Spec.NodeSelector)
	if err != nil {
		return err
	}

	s := mapset.NewSet[string]()
	if err := s.UnmarshalJSON([]byte(vc.Annotations[utils.KeyMatchedNodes])); err != nil {
		return err
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
