package nad

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/harvester/webhook/pkg/types"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

var _ types.Mutator = &NadMutator{}

type NadMutator struct {
	types.DefaultMutator
	cnCache ctlnetworkv1.ClusterNetworkCache
}

func NewNadMutator(cnCache ctlnetworkv1.ClusterNetworkCache) *NadMutator {
	return &NadMutator{
		cnCache: cnCache,
	}
}

func (n *NadMutator) Update(_ *types.Request, oldObj, newObj runtime.Object) (types.Patch, error) {
	oldNad := oldObj.(*cniv1.NetworkAttachmentDefinition)
	newNad := newObj.(*cniv1.NetworkAttachmentDefinition)

	if newNad.DeletionTimestamp != nil {
		return nil, nil
	}

	if oldNad.Spec.Config == newNad.Spec.Config {
		return nil, nil
	}

	oldNetconf, newNetconf := &utils.NetConf{}, &utils.NetConf{}
	if err := json.Unmarshal([]byte(oldNad.Spec.Config), oldNetconf); err != nil {
		return nil, fmt.Errorf(updateErr, oldNad.Namespace, oldNad.Name, err)
	}
	if err := json.Unmarshal([]byte(newNad.Spec.Config), newNetconf); err != nil {
		return nil, fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	patch, err := n.ensureLabels(newNad, oldNetconf, newNetconf)
	if err != nil {
		return nil, fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	annotationPatch, err := tagRouteOutdated(newNad, oldNetconf, newNetconf)
	if err != nil {
		return nil, fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	return append(patch, annotationPatch...), nil
}

func (n *NadMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"network-attachment-definitions"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   cniv1.SchemeGroupVersion.Group,
		APIVersion: cniv1.SchemeGroupVersion.Version,
		ObjectType: &cniv1.NetworkAttachmentDefinition{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Update,
		},
	}
}

func (n *NadMutator) ensureLabels(nad *cniv1.NetworkAttachmentDefinition, oldConf, newConf *utils.NetConf) (types.Patch, error) {
	labels := nad.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	if newConf.Vlan != 0 {
		labels[utils.KeyNetworkType] = string(utils.L2VlanNetwork)
		labels[utils.KeyVlanLabel] = strconv.Itoa(newConf.Vlan)
	} else {
		labels[utils.KeyNetworkType] = string(utils.UntaggedNetwork)
		delete(labels, utils.KeyVlanLabel)
	}
	if oldConf.Vlan != 0 && oldConf.Vlan != newConf.Vlan {
		labels[utils.KeyLastVlanLabel] = strconv.Itoa(oldConf.Vlan)
	}

	cnName := newConf.BrName[:len(newConf.BrName)-len(iface.BridgeSuffix)]
	labels[utils.KeyClusterNetworkLabel] = cnName
	if oldConf.BrName != newConf.BrName {
		labels[utils.KeyLastClusterNetworkLabel] = oldConf.BrName[:len(oldConf.BrName)-len(iface.BridgeSuffix)]
	}
	if cn, err := n.cnCache.Get(cnName); err != nil {
		return nil, err
	} else if networkv1.Ready.IsTrue(cn.Status) {
		labels[utils.KeyNetworkReady] = utils.ValueTrue
	} else {
		labels[utils.KeyNetworkReady] = utils.ValueFalse
	}

	return types.Patch{
		types.PatchOp{
			Op:    types.PatchOpReplace,
			Path:  "/metadata/labels",
			Value: labels,
		}}, nil
}

func tagRouteOutdated(nad *cniv1.NetworkAttachmentDefinition, oldConf, newConf *utils.NetConf) (types.Patch, error) {
	if oldConf.BrName == newConf.BrName && oldConf.Vlan == newConf.Vlan {
		return nil, nil
	}

	annotations := nad.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	klog.Infof("new config: %+v", newConf)

	if newConf.Vlan == 0 {
		delete(annotations, utils.KeyNetworkRoute)
	} else {
		layer3NetworkConf := utils.Layer3NetworkConf{}
		routeAnnotation := nad.Annotations[utils.KeyNetworkRoute]
		if err := json.Unmarshal([]byte(routeAnnotation), &layer3NetworkConf); err != nil {
			return nil, err
		}

		if layer3NetworkConf.Mode != utils.Auto {
			return nil, nil
		}

		layer3NetworkConf.Outdated = true

		outdatedRoute, err := json.Marshal(layer3NetworkConf)
		if err != nil {
			return nil, err
		}
		annotations[utils.KeyNetworkRoute] = string(outdatedRoute)
	}

	return types.Patch{
		types.PatchOp{
			Op:    types.PatchOpReplace,
			Path:  "/metadata/annotations",
			Value: annotations,
		},
	}, nil
}
