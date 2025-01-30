package vlanconfig

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/harvester/webhook/pkg/server/admission"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester/pkg/util"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubevirt.io/v1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	createErr                = "could not create vlanConfig %s because %w"
	updateErr                = "could not update vlanConfig %s because %w"
	deleteErr                = "could not delete vlanConfig %s because %w"
	StorageNetworkAnnotation = "storage-network.settings.harvesterhci.io"
)

type Validator struct {
	admission.DefaultValidator

	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
	vcCache  ctlnetworkv1.VlanConfigCache
	vsCache  ctlnetworkv1.VlanStatusCache
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache
}

func NewVlanConfigValidator(
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache,
	vcCache ctlnetworkv1.VlanConfigCache,
	vsCache ctlnetworkv1.VlanStatusCache,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) *Validator {
	return &Validator{
		nadCache: nadCache,
		vcCache:  vcCache,
		vsCache:  vsCache,
		vmiCache: vmiCache,
	}
}

var _ admission.Validator = &Validator{}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	vc := newObj.(*networkv1.VlanConfig)

	if vc.Spec.ClusterNetwork == utils.ManagementClusterNetworkName {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("cluster network could not be %s",
			utils.ManagementClusterNetworkName))
	}

	if err := v.validateMTU(vc); err != nil {
		return fmt.Errorf(createErr, vc.Name, err)
	}

	nodes, err := getMatchNodes(vc)
	if err != nil {
		return fmt.Errorf(createErr, vc.Name, err)
	}

	if err := v.checkOverlaps(vc, nodes); err != nil {
		return fmt.Errorf(createErr, vc.Name, err)
	}

	maxClusterNetworkNameLen := iface.MaxDeviceNameLen - len(iface.BridgeSuffix)

	if len(vc.Spec.ClusterNetwork) > maxClusterNetworkNameLen {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("the length of the clusterNetwork value is "+
			"more than %d", maxClusterNetworkNameLen))
	}

	return nil
}

func (v *Validator) Update(_ *admission.Request, oldObj, newObj runtime.Object) error {
	oldVc := oldObj.(*networkv1.VlanConfig)
	newVc := newObj.(*networkv1.VlanConfig)

	if newVc.Spec.ClusterNetwork == utils.ManagementClusterNetworkName {
		return fmt.Errorf(updateErr, newVc.Name, fmt.Errorf("cluster network could not be %s",
			utils.ManagementClusterNetworkName))
	}
	// skip validation if spec is not changed
	if reflect.DeepEqual(oldVc.Spec, newVc.Spec) {
		return nil
	}

	if err := v.validateMTU(newVc); err != nil {
		return fmt.Errorf(updateErr, newVc.Name, err)
	}

	newNodes, err := getMatchNodes(newVc)
	if err != nil {
		return fmt.Errorf(updateErr, newVc.Name, err)
	}

	if err := v.checkOverlaps(newVc, newNodes); err != nil {
		return fmt.Errorf(updateErr, newVc.Name, err)
	}

	oldNodes, err := getMatchNodes(oldVc)
	if err != nil {
		return fmt.Errorf(updateErr, oldVc.Name, err)
	}

	// get affected nodes after updating
	affectedNodes := getAffectedNodes(oldVc.Spec.ClusterNetwork, newVc.Spec.ClusterNetwork, oldNodes, newNodes)
	if err := v.checkVmi(oldVc, affectedNodes); err != nil {
		return fmt.Errorf(updateErr, oldVc.Name, err)
	}

	return nil
}

func getAffectedNodes(oldCn, newCn string, oldNodes, newNodes mapset.Set[string]) mapset.Set[string] {
	if newCn != oldCn {
		return oldNodes
	}

	return oldNodes.Difference(newNodes)
}

func (v *Validator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	vc := oldObj.(*networkv1.VlanConfig)

	nodes, err := getMatchNodes(vc)
	if err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	if err := v.checkVmi(vc, nodes); err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	nads, err := v.nadCache.List(util.HarvesterSystemNamespaceName, labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: vc.Spec.ClusterNetwork,
	}).AsSelector())
	if err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	if len(nads) > 0 {
		for _, nad := range nads {
			if nad.DeletionTimestamp == nil && nad.Annotations[StorageNetworkAnnotation] == "true" {
				return fmt.Errorf(deleteErr, vc.Name, fmt.Errorf(`storage network nad %s is still attached`, nad.Name))
			}
		}
	}

	return nil
}

func (v *Validator) Resource() admission.Resource {
	return admission.Resource{
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

func (v *Validator) checkOverlaps(vc *networkv1.VlanConfig, nodes mapset.Set[string]) error {
	overlapNods := mapset.NewSet[string]()
	for node := range nodes.Iter() {
		vsName := utils.Name("", vc.Spec.ClusterNetwork, node)
		if vs, err := v.vsCache.Get(vsName); err != nil && !apierrors.IsNotFound(err) {
			return err
		} else if err == nil && vs.Status.VlanConfig != vc.Name {
			// The vlanconfig is found means a vlanconfig with the same cluster network has been taken effect on this node.
			overlapNods.Add(node)
		}
	}

	if overlapNods.Cardinality() > 0 {
		return fmt.Errorf("it overlaps with other vlanconfigs matching node(s) %v", overlapNods.ToSlice())
	}

	return nil
}

// checkVmi is to confirm if any VMIs will be affected on affected nodes. Those VMIs must be stopped in advance.
func (v *Validator) checkVmi(vc *networkv1.VlanConfig, nodes mapset.Set[string]) error {
	// The vlanconfig is not allowed to be deleted if it has applied to some nodes and its clusternetwork is attached by some nads.
	vss, err := v.vsCache.List(labels.Set(map[string]string{utils.KeyVlanConfigLabel: vc.Name}).AsSelector())
	if err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	if len(vss) == 0 {
		return nil
	}

	nads, err := v.nadCache.List("", labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: vc.Spec.ClusterNetwork,
	}).AsSelector())
	if err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	vmiGetter := utils.VmiGetter{VmiCache: v.vmiCache}
	vmis := make([]*kubevirtv1.VirtualMachineInstance, 0)
	for _, nad := range nads {
		vmisTemp, err := vmiGetter.WhoUseNad(nad, nodes)
		if err != nil {
			return err
		}
		vmis = append(vmis, vmisTemp...)
	}

	if len(vmis) > 0 {
		vmiStrList := make([]string, len(vmis))
		for i, vmi := range vmis {
			vmiStrList[i] = vmi.Namespace + "/" + vmi.Name
		}

		return fmt.Errorf("it's blocked by VM(s) %s which must be stopped at first", strings.Join(vmiStrList, ", "))
	}

	return nil
}

func getMatchNodes(vc *networkv1.VlanConfig) (mapset.Set[string], error) {
	if vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" {
		return nil, nil
	}

	var matchedNodes []string
	if err := json.Unmarshal([]byte(vc.Annotations[utils.KeyMatchedNodes]), &matchedNodes); err != nil {
		return nil, err
	}

	return mapset.NewSet[string](matchedNodes...), nil
}

func (v *Validator) validateMTU(current *networkv1.VlanConfig) error {
	vcs, err := v.vcCache.List(labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: current.Spec.ClusterNetwork,
	}).AsSelector())
	if err != nil {
		return err
	}

	for _, vc := range vcs {
		if vc.Name == current.Name {
			continue
		}
		if current.Spec.Uplink.LinkAttrs.MTU != vc.Spec.Uplink.LinkAttrs.MTU {
			return fmt.Errorf("the MTU is different from network config %s", vc.Name)
		}
	}

	return nil
}
