package node

import (
	"context"

	"github.com/deckarep/golang-set/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const controllerName = "harvester-network-manager-node-controller"

type Handler struct {
	vcCache  ctlnetworkv1.VlanConfigCache
	vcClient ctlnetworkv1.VlanConfigClient
}

func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	vcs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()

	h := Handler{
		vcClient: vcs,
		vcCache:  vcs.Cache(),
	}

	nodes.OnChange(ctx, controllerName, h.OnChange)

	return nil
}

func (h Handler) OnChange(_ string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return nil, nil
	}

	vcs, err := h.vcCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, vc := range vcs {
		if err := h.updateMatchedNodeAnnotation(vc, node); err != nil {
			return nil, err
		}
	}

	return nil, nil
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
