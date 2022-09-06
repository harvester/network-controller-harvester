package vlanconfig

import (
	"encoding/json"
	"fmt"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/yaocw2020/webhook/pkg/types"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type vlanConfigMutator struct {
	types.DefaultMutator

	nodeCache ctlcorev1.NodeCache
}

var _ types.Mutator = &vlanConfigMutator{}

func NewNadMutator(nodeCache ctlcorev1.NodeCache) *vlanConfigMutator {
	return &vlanConfigMutator{
		nodeCache: nodeCache,
	}
}

func (v *vlanConfigMutator) Create(_ *types.Request, newObj runtime.Object) (types.Patch, error) {
	vlanConfig := newObj.(*networkv1.VlanConfig)

	annotationPatch, err := v.matchNodes(vlanConfig)
	if err != nil {
		return nil, fmt.Errorf(createErr, vlanConfig.Name, err)
	}

	return append(getCnLabelPatch(vlanConfig), annotationPatch...), nil
}

func (v *vlanConfigMutator) Update(_ *types.Request, oldObj, newObj runtime.Object) (types.Patch, error) {
	newVc := newObj.(*networkv1.VlanConfig)
	oldVc := newObj.(*networkv1.VlanConfig)

	var cnLabelPatch, annotationPatch types.Patch
	if newVc.Spec.ClusterNetwork != oldVc.Spec.ClusterNetwork {
		cnLabelPatch = getCnLabelPatch(newVc)
	}

	if !reflect.DeepEqual(newVc.Spec.NodeSelector, oldVc.Spec.NodeSelector) {
		var err error
		annotationPatch, err = v.matchNodes(newVc)
		if err != nil {
			return nil, fmt.Errorf(updateErr, newVc.Name, err)
		}
	}

	return append(cnLabelPatch, annotationPatch...), nil
}

func getCnLabelPatch(v *networkv1.VlanConfig) types.Patch {
	if v.Labels[utils.KeyClusterNetworkLabel] == v.Spec.ClusterNetwork {
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

func (v *vlanConfigMutator) matchNodes(vc *networkv1.VlanConfig) (types.Patch, error) {
	selector, err := utils.NewSelector(vc.Spec.NodeSelector)
	if err != nil {
		return nil, err
	}
	nodes, err := v.nodeCache.List(selector)
	if err != nil {
		return nil, err
	}

	matchedNodes := make([]string, len(nodes))
	for i, node := range nodes {
		matchedNodes[i] = node.Name
	}
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

func (v *vlanConfigMutator) Resource() types.Resource {
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
