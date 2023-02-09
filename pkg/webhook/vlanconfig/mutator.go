package vlanconfig

import (
	"encoding/json"
	"fmt"

	"github.com/harvester/webhook/pkg/types"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type Mutator struct {
	types.DefaultMutator

	nodeCache ctlcorev1.NodeCache
}

var _ types.Mutator = &Mutator{}

func NewVlanConfigMutator(nodeCache ctlcorev1.NodeCache) *Mutator {
	return &Mutator{
		nodeCache: nodeCache,
	}
}

func (m *Mutator) Create(_ *types.Request, newObj runtime.Object) (types.Patch, error) {
	vlanConfig := newObj.(*networkv1.VlanConfig)

	annotationPatch, err := m.matchNodes(vlanConfig)
	if err != nil {
		return nil, fmt.Errorf(createErr, vlanConfig.Name, err)
	}

	return append(getCnLabelPatch(vlanConfig), annotationPatch...), nil
}

func (m *Mutator) Update(_ *types.Request, oldObj, newObj runtime.Object) (types.Patch, error) {
	newVc := newObj.(*networkv1.VlanConfig)
	oldVc := oldObj.(*networkv1.VlanConfig)

	var cnLabelPatch, annotationPatch types.Patch
	if newVc.Spec.ClusterNetwork != oldVc.Spec.ClusterNetwork {
		cnLabelPatch = getCnLabelPatch(newVc)
	}

	annotationPatch, err := m.matchNodes(newVc)
	if err != nil {
		return nil, fmt.Errorf(updateErr, newVc.Name, err)
	}

	return append(cnLabelPatch, annotationPatch...), nil
}

func getCnLabelPatch(v *networkv1.VlanConfig) types.Patch {
	if v.Labels != nil && v.Labels[utils.KeyClusterNetworkLabel] == v.Spec.ClusterNetwork {
		return nil
	}

	labels := v.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[utils.KeyClusterNetworkLabel] = v.Spec.ClusterNetwork

	return types.Patch{
		types.PatchOp{
			Op:    types.PatchOpReplace,
			Path:  "/metadata/labels",
			Value: labels,
		}}
}

func (m *Mutator) matchNodes(vc *networkv1.VlanConfig) (types.Patch, error) {
	nodes, err := m.nodeCache.List(labels.Set(vc.Spec.NodeSelector).AsSelector())
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	matchedNodes := make([]string, 0, len(nodes))
	for _, node := range nodes {
		matchedNodes = append(matchedNodes, node.Name)
	}

	return matchedNodesToPatch(vc, matchedNodes)
}

func matchedNodesToPatch(vc *networkv1.VlanConfig, matchedNodes []string) (types.Patch, error) {
	annotations := vc.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}

	nodesBytes, err := json.Marshal(matchedNodes)
	if err != nil {
		return nil, err
	}
	annotations[utils.KeyMatchedNodes] = string(nodesBytes)

	return types.Patch{
		types.PatchOp{
			Op:    types.PatchOpReplace,
			Path:  "/metadata/annotations",
			Value: annotations,
		},
	}, nil
}

func (m *Mutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"vlanconfigs"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   networkv1.SchemeGroupVersion.Group,
		APIVersion: networkv1.SchemeGroupVersion.Version,
		ObjectType: &networkv1.VlanConfig{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}
