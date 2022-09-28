package vlanconfig

import (
	"encoding/json"
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/yaocw2020/webhook/pkg/types"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type vlanConfigMutator struct {
	types.DefaultMutator

	nodeCache ctlcorev1.NodeCache
	vsCache   ctlnetworkv1.VlanStatusCache
}

var _ types.Mutator = &vlanConfigMutator{}

func NewNadMutator(nodeCache ctlcorev1.NodeCache, vsCache ctlnetworkv1.VlanStatusCache) *vlanConfigMutator {
	return &vlanConfigMutator{
		nodeCache: nodeCache,
		vsCache:   vsCache,
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

	annotationPatch, err := v.matchNodes(newVc)
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

func (v *vlanConfigMutator) matchNodes(vc *networkv1.VlanConfig) (types.Patch, error) {
	nodes, err := v.nodeCache.List(labels.Set(vc.Spec.NodeSelector).AsSelector())
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	matchedNodes, pausedNodes := make([]string, 0, len(nodes)), make([]string, 0, len(nodes))
	for _, node := range nodes {
		vsName := utils.Name("", vc.Spec.ClusterNetwork, node.Name)
		if vs, err := v.vsCache.Get(vsName); err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		} else if err == nil && vs.Status.VlanConfig != vc.Name {
			// The vlanconfig is found means a vlanconfig with the same clusternetwork
			// has been taken effect on this node. We have to pause the new vlanconfig.
			pausedNodes = append(pausedNodes, node.Name)
		}
		matchedNodes = append(matchedNodes, node.Name)
	}

	return matchedNodesToPatch(vc, matchedNodes, pausedNodes)
}

func matchedNodesToPatch(vc *networkv1.VlanConfig, matchedNodes, pausedNodes []string) (types.Patch, error) {
	annotations := vc.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	nodesBytes, err := json.Marshal(matchedNodes)
	if err != nil {
		return nil, err
	}
	annotations[utils.KeyMatchedNodes] = string(nodesBytes)
	pausedNodesBytes, err := json.Marshal(pausedNodes)
	if err != nil {
		return nil, err
	}
	annotations[utils.KeyPausedNodes] = string(pausedNodesBytes)
	// add matched-nodes and paused-nodes annotations
	patch := types.Patch{
		types.PatchOp{
			Op:    types.PatchOpReplace,
			Path:  "/metadata/annotations",
			Value: annotations,
		},
	}
	// modify paused to be true
	if len(pausedNodes) > 0 {
		patch = append(patch, types.PatchOp{
			Op:    types.PatchOpReplace,
			Path:  "/spec/paused",
			Value: true,
		})
	}

	return patch, nil
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
