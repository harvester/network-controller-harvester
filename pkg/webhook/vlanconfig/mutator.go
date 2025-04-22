package vlanconfig

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/harvester/webhook/pkg/server/admission"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type Mutator struct {
	admission.DefaultMutator

	nodeCache ctlcorev1.NodeCache
}

var _ admission.Mutator = &Mutator{}

func NewVlanConfigMutator(nodeCache ctlcorev1.NodeCache) *Mutator {
	return &Mutator{
		nodeCache: nodeCache,
	}
}

func (m *Mutator) Create(_ *admission.Request, newObj runtime.Object) (admission.Patch, error) {
	vlanConfig := newObj.(*networkv1.VlanConfig)

	annotationPatch, err := m.matchNodes(vlanConfig)
	if err != nil {
		return nil, fmt.Errorf(createErr, vlanConfig.Name, err)
	}

	return append(getCnLabelPatch(vlanConfig), annotationPatch...), nil
}

func (m *Mutator) Update(_ *admission.Request, oldObj, newObj runtime.Object) (admission.Patch, error) {
	newVc := newObj.(*networkv1.VlanConfig)
	oldVc := oldObj.(*networkv1.VlanConfig)

	// skip mutation if spec is not changed
	if reflect.DeepEqual(oldVc.Spec, newVc.Spec) {
		return nil, nil
	}

	var cnLabelPatch, annotationPatch admission.Patch
	if newVc.Spec.ClusterNetwork != oldVc.Spec.ClusterNetwork {
		cnLabelPatch = getCnLabelPatch(newVc)
	}

	annotationPatch, err := m.matchNodes(newVc)
	if err != nil {
		return nil, fmt.Errorf(updateErr, newVc.Name, err)
	}

	return append(cnLabelPatch, annotationPatch...), nil
}

func getCnLabelPatch(v *networkv1.VlanConfig) admission.Patch {
	if v.Labels != nil && v.Labels[utils.KeyClusterNetworkLabel] == v.Spec.ClusterNetwork {
		return nil
	}

	labels := v.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[utils.KeyClusterNetworkLabel] = v.Spec.ClusterNetwork

	return admission.Patch{
		admission.PatchOp{
			Op:    admission.PatchOpReplace,
			Path:  "/metadata/labels",
			Value: labels,
		}}
}

func (m *Mutator) matchNodes(vc *networkv1.VlanConfig) (admission.Patch, error) {
	selector := labels.Set(vc.Spec.NodeSelector).AsSelector()
	witnessFilter, err := labels.NewRequirement(utils.HarvesterWitnessNodeLabelKey, selection.DoesNotExist, nil)
	if err != nil {
		return nil, err
	}
	selector = selector.Add(*witnessFilter)
	nodes, err := m.nodeCache.List(selector)
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

func matchedNodesToPatch(vc *networkv1.VlanConfig, matchedNodes []string) (admission.Patch, error) {
	annotations := vc.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}

	nodesBytes, err := json.Marshal(matchedNodes)
	if err != nil {
		return nil, err
	}
	annotations[utils.KeyMatchedNodes] = string(nodesBytes)

	return admission.Patch{
		admission.PatchOp{
			Op:    admission.PatchOpReplace,
			Path:  "/metadata/annotations",
			Value: annotations,
		},
	}, nil
}

func (m *Mutator) Resource() admission.Resource {
	return admission.Resource{
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
