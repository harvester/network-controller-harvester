package nad

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"github.com/harvester/webhook/pkg/server/admission"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/tidwall/sjson"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

var _ admission.Mutator = &Mutator{}

type Mutator struct {
	admission.DefaultMutator
	cnCache ctlnetworkv1.ClusterNetworkCache
	vcCache ctlnetworkv1.VlanConfigCache
}

func NewNadMutator(cnCache ctlnetworkv1.ClusterNetworkCache,
	vcCache ctlnetworkv1.VlanConfigCache) *Mutator {
	return &Mutator{
		cnCache: cnCache,
		vcCache: vcCache,
	}
}

func (m *Mutator) Create(_ *admission.Request, newObj runtime.Object) (admission.Patch, error) {
	nad := newObj.(*cniv1.NetworkAttachmentDefinition)

	patch, err := m.patchMTU(nad)
	if err != nil {
		return nil, fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}

	return patch, nil
}

func (m *Mutator) Update(_ *admission.Request, oldObj, newObj runtime.Object) (admission.Patch, error) {
	oldNad := oldObj.(*cniv1.NetworkAttachmentDefinition)
	newNad := newObj.(*cniv1.NetworkAttachmentDefinition)

	if newNad.DeletionTimestamp != nil {
		return nil, nil
	}

	oldNetconf, newNetconf := &utils.NetConf{}, &utils.NetConf{}
	if err := json.Unmarshal([]byte(oldNad.Spec.Config), oldNetconf); err != nil {
		return nil, fmt.Errorf(updateErr, oldNad.Namespace, oldNad.Name, err)
	}
	if err := json.Unmarshal([]byte(newNad.Spec.Config), newNetconf); err != nil {
		return nil, fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}
	// ignore the update if the config is not being updated
	if reflect.DeepEqual(oldNetconf, newNetconf) {
		return nil, nil
	}

	patch, err := m.ensureLabels(newNad, oldNetconf, newNetconf)
	if err != nil {
		return nil, fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	annotationPatch, err := tagRouteOutdated(newNad, oldNetconf, newNetconf)
	if err != nil {
		return nil, fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	return append(patch, annotationPatch...), nil
}

func (m *Mutator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"network-attachment-definitions"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   cniv1.SchemeGroupVersion.Group,
		APIVersion: cniv1.SchemeGroupVersion.Version,
		ObjectType: &cniv1.NetworkAttachmentDefinition{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
		},
	}
}

func (m *Mutator) ensureLabels(nad *cniv1.NetworkAttachmentDefinition, oldConf, newConf *utils.NetConf) (admission.Patch, error) {
	labels := nad.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	if newConf.Type == utils.CNITypeKubeOVN {
		labels[utils.KeyNetworkType] = string(utils.OverlayNetwork)
		labels[utils.KeyNetworkReady] = utils.ValueTrue
		if labels[utils.KeyClusterNetworkLabel] == "" {
			labels[utils.KeyClusterNetworkLabel] = utils.ManagementClusterNetworkName
		}
		return admission.Patch{
			admission.PatchOp{
				Op:    admission.PatchOpReplace,
				Path:  "/metadata/labels",
				Value: labels,
			}}, nil
	}

	// Ignore untagged network because we don't need to do more operation if the last network type is untagged network
	if oldConf.Vlan != 0 && newConf.Vlan == 0 {
		labels[utils.KeyLastNetworkType] = string(utils.L2VlanNetwork)
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
	if cn, err := m.cnCache.Get(cnName); err != nil {
		return nil, err
	} else if networkv1.Ready.IsTrue(cn.Status) {
		labels[utils.KeyNetworkReady] = utils.ValueTrue
	} else {
		labels[utils.KeyNetworkReady] = utils.ValueFalse
	}

	return admission.Patch{
		admission.PatchOp{
			Op:    admission.PatchOpReplace,
			Path:  "/metadata/labels",
			Value: labels,
		}}, nil
}

// If the vlan or bridge name is changed, we need to tag the route annotation outdated
func tagRouteOutdated(nad *cniv1.NetworkAttachmentDefinition, oldConf, newConf *utils.NetConf) (admission.Patch, error) {
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
		routeAnnotation := annotations[utils.KeyNetworkRoute]
		if err := json.Unmarshal([]byte(routeAnnotation), &layer3NetworkConf); routeAnnotation != "" && err != nil {
			return nil, fmt.Errorf("unmarshal %s failed, error: %w", routeAnnotation, err)
		}

		if layer3NetworkConf.Mode != utils.Auto {
			return nil, nil
		}

		layer3NetworkConf.Outdated = true

		outdatedRoute, err := json.Marshal(layer3NetworkConf)
		if err != nil {
			return nil, fmt.Errorf("marshal %v failed, error: %w", layer3NetworkConf, err)
		}
		annotations[utils.KeyNetworkRoute] = string(outdatedRoute)
	}

	return admission.Patch{
		admission.PatchOp{
			Op:    admission.PatchOpReplace,
			Path:  "/metadata/annotations",
			Value: annotations,
		},
	}, nil
}

func (m *Mutator) patchMTU(nad *cniv1.NetworkAttachmentDefinition) (admission.Patch, error) {
	config := nad.Spec.Config
	netConf := &utils.NetConf{}
	if err := json.Unmarshal([]byte(config), netConf); err != nil {
		return nil, err
	}

	if netConf.Type == utils.CNITypeKubeOVN {
		return nil, nil
	}

	clusterNetwork := netConf.BrName[:len(netConf.BrName)-len(iface.BridgeSuffix)]
	cn, err := m.cnCache.Get(clusterNetwork)
	if err != nil {
		return nil, err
	}

	targetMTU := utils.DefaultMTU
	getMtu := false

	// get MTU from clusternetwork
	if mtuStr, ok := cn.Annotations[utils.KeyUplinkMTU]; ok {
		mtu, err := utils.GetMTUFromAnnotation(mtuStr)
		if err != nil {
			return nil, fmt.Errorf("nad's host cluster network %v has invalid MTU annotation %v/%v %w", cn.Name, utils.KeyUplinkMTU, mtuStr, err)
		}
		if mtu != 0 {
			targetMTU = mtu
		}
		getMtu = true
	}

	// get MTU value from vlanconfig
	if !getMtu {
		vcs, err := m.vcCache.List(k8slabels.Set(map[string]string{
			utils.KeyClusterNetworkLabel: clusterNetwork,
		}).AsSelector())
		if err != nil {
			return nil, err
		}

		// if there is no vlanconfig, use default; in other case, use the first one
		if len(vcs) != 0 {
			vcMtu := utils.GetMTUFromVlanConfig(vcs[0])
			if utils.IsValidMTU(vcMtu) && vcMtu != 0 {
				targetMTU = vcMtu
			}
		}
	}

	if utils.AreEqualMTUs(targetMTU, netConf.MTU) {
		return nil, nil
	}

	klog.Infof("nad %s/%s MTU is patched from %v to %v", nad.Namespace, nad.Name, netConf.MTU, targetMTU)
	// Don't modify the unmarshalled structure and marshal it again because some fields may be lost during unmarshalling.
	newConfig, err := sjson.Set(config, "mtu", targetMTU)
	if err != nil {
		return nil, fmt.Errorf("set mtu failed, error: %w", err)
	}

	return admission.Patch{
		admission.PatchOp{
			Op:    admission.PatchOpReplace,
			Path:  "/spec/config",
			Value: newConfig,
		},
	}, nil
}
