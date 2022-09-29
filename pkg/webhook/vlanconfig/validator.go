package vlanconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"github.com/yaocw2020/webhook/pkg/types"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	createErr = "could not create vlanConfig %s because %w"
	updateErr = "could not update vlanConfig %s because %w"
	deleteErr = "could not delete vlanConfig %s because %w"
)

type vlanConfigValidator struct {
	types.DefaultValidator
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
	vsCache  ctlnetworkv1.VlanStatusCache
}

func NewVlanConfigValidator(nadCache ctlcniv1.NetworkAttachmentDefinitionCache,
	vsCache ctlnetworkv1.VlanStatusCache) *vlanConfigValidator {
	return &vlanConfigValidator{
		nadCache: nadCache,
		vsCache:  vsCache,
	}
}

var _ types.Validator = &vlanConfigValidator{}

func (v *vlanConfigValidator) Create(_ *types.Request, newObj runtime.Object) error {
	vc := newObj.(*networkv1.VlanConfig)

	if vc.Spec.ClusterNetwork == utils.ManagementClusterNetworkName {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("cluster network could not be %s",
			utils.ManagementClusterNetworkName))
	}

	if err := v.checkOverlaps(vc); err != nil {
		return fmt.Errorf(createErr, vc.Name, err)
	}

	maxClusterNetworkNameLen := iface.MaxDeviceNameLen - len(iface.BridgeSuffix)

	if len(vc.Spec.ClusterNetwork) > maxClusterNetworkNameLen {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("the length of the clusterNetwork value is "+
			"more than %d", maxClusterNetworkNameLen))
	}

	return nil
}

func (v *vlanConfigValidator) Update(_ *types.Request, oldObj, newObj runtime.Object) error {
	vc := newObj.(*networkv1.VlanConfig)

	if vc.Spec.ClusterNetwork == utils.ManagementClusterNetworkName {
		return fmt.Errorf(updateErr, vc.Name, fmt.Errorf("cluster network could not be %s",
			utils.ManagementClusterNetworkName))
	}

	if err := v.checkOverlaps(vc); err != nil {
		return fmt.Errorf(updateErr, vc.Name, err)
	}

	return nil
}

func (v *vlanConfigValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	vc := oldObj.(*networkv1.VlanConfig)
	// The vlanconfig is not allowed to be deleted if it has applied to some nodes and its clusternetwork is attached by
	// some nads.
	vss, err := v.vsCache.List(labels.Set(map[string]string{
		utils.KeyVlanConfigLabel: vc.Name,
	}).AsSelector())
	if err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	nads, err := v.nadCache.List("", labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: vc.Spec.ClusterNetwork,
	}).AsSelector())
	if err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	if len(vss) > 0 && len(nads) > 0 {
		nadStrList := make([]string, len(nads))
		for i, nad := range nads {
			nadStrList[i] = nad.Namespace + "/" + nad.Name
		}
		return fmt.Errorf(deleteErr, vc.Name, fmt.Errorf("it's not allowed to delete the vlanconfig %s "+
			"because these nads %s are working", vc.Name, strings.Join(nadStrList, ",")))
	}

	return nil
}

func (v *vlanConfigValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"vlanconfigs"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   networkv1.SchemeGroupVersion.Group,
		APIVersion: networkv1.SchemeGroupVersion.Version,
		ObjectType: &networkv1.VlanConfig{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

func (v *vlanConfigValidator) checkOverlaps(vc *networkv1.VlanConfig) error {
	var matchedNodes []string
	if vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(vc.Annotations[utils.KeyMatchedNodes]), &matchedNodes); err != nil {
		return err
	}
	if len(matchedNodes) == 0 {
		return nil
	}

	pausedNodes := make([]string, 0, len(matchedNodes))
	for _, node := range matchedNodes {
		vsName := utils.Name("", vc.Spec.ClusterNetwork, node)
		if vs, err := v.vsCache.Get(vsName); err != nil && !apierrors.IsNotFound(err) {
			return err
		} else if err == nil && vs.Status.VlanConfig != vc.Name {
			// The vlanconfig is found means a vlanconfig with the same clusternetwork  has been taken effect on this node.
			pausedNodes = append(pausedNodes, node)
		}
	}

	if len(pausedNodes) > 0 {
		return fmt.Errorf("it overlaps with other vlanconfigs matching node(s) %v", pausedNodes)
	}

	return nil
}
