package nad

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/harvester/webhook/pkg/server/admission"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/tidwall/sjson"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
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

	oldNetconf, err := utils.DecodeNadConfigToNetConf(oldNad)
	if err != nil {
		return nil, fmt.Errorf(updateErr, oldNad.Namespace, oldNad.Name, err)
	}

	newNetconf, err := utils.DecodeNadConfigToNetConf(newNad)
	if err != nil {
		return nil, fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	patch, err := m.ensureLabels(newNad, oldNetconf, newNetconf)
	if err != nil {
		return nil, fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	annotationPatch, err := tagRouteOutdated(oldNad, newNad, oldNetconf, newNetconf)
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

func (m *Mutator) ensureLabels(nad *cniv1.NetworkAttachmentDefinition, _, newConf *utils.NetConf) (admission.Patch, error) {
	labels := nad.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	if newConf.IsKubeOVNCNI() {
		labels[utils.KeyNetworkType] = string(utils.OverlayNetwork)
		labels[utils.KeyNetworkReady] = utils.ValueTrue
		if labels[utils.KeyClusterNetworkLabel] == "" {
			labels[utils.KeyClusterNetworkLabel] = utils.ManagementClusterNetworkName
		}
		delete(labels, utils.KeyVlanLabel)
		return admission.Patch{
			admission.PatchOp{
				Op:    admission.PatchOpReplace,
				Path:  "/metadata/labels",
				Value: labels,
			}}, nil
	}

	err := newConf.SetNetworkInfoToLabels(labels)
	if err != nil {
		return nil, err
	}

	// check the related clusternetwork is there
	cnName := labels[utils.KeyClusterNetworkLabel]
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

// If the vlan/route mode is changed, we need to tag the route annotation outdated
func tagRouteOutdated(oldNad, newNad *cniv1.NetworkAttachmentDefinition, oldConf, newConf *utils.NetConf) (admission.Patch, error) {
	if newConf.IsKubeOVNCNI() {
		return nil, nil
	}
	// even when vlan is not changed, the route mode can be changed from `auto` to `static` or vice versa
	if len(newNad.Annotations) == 0 {
		// nothing to remove/update, skip
		return nil, nil
	}

	if oldConf.Vlan != newConf.Vlan {
		logrus.Infof("nad %s/%s has new config, route is updated: %+v", newNad.Namespace, newNad.Name, newConf)
		annotations, err := utils.OutdateNadLayer3NetworkConf(newNad, newConf)
		if err != nil {
			return nil, err
		}

		return admission.Patch{
			admission.PatchOp{
				Op:    admission.PatchOpReplace,
				Path:  "/metadata/annotations",
				Value: annotations,
			},
		}, nil
	}

	// a corner case: when nad's route is changed from `auto` to static or vice versa
	annotations, err := utils.OutdateNadLayer3NetworkConfWhenRouteModeChanges(oldNad, newNad)
	if err != nil {
		return nil, err
	}
	// if no change, the returned annotations is nil
	if annotations == nil {
		return nil, nil
	}
	logrus.Infof("nad %s/%s has new route mode, route is updated: %s", newNad.Namespace, newNad.Name, newNad.Annotations[utils.KeyNetworkRoute])
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

	netConf, err := utils.DecodeNadConfigToNetConf(nad)
	if err != nil {
		return nil, err
	}

	if netConf.IsKubeOVNCNI() {
		return nil, nil
	}

	clusterNetwork, err := utils.GetClusterNetworkFromBridgeName(netConf.BrName)
	if err != nil {
		return nil, err
	}

	cn, err := m.cnCache.Get(clusterNetwork)
	if err != nil {
		return nil, err
	}

	targetMTU := utils.DefaultMTU
	getMtu := false

	// get MTU from clusternetwork
	if mtuStr, ok := cn.Annotations[utils.KeyUplinkMTU]; ok {
		mtu, err := utils.GetMTUFromString(mtuStr)
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

	logrus.Infof("nad %s/%s MTU is patched from %v to %v", nad.Namespace, nad.Name, netConf.MTU, targetMTU)
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
