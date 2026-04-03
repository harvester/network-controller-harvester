package vlanconfig

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/harvester/webhook/pkg/server/admission"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubevirt.io/v1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	createErr = "can't create vlanConfig %s because %w"
	updateErr = "can't update vlanConfig %s because %w"
	deleteErr = "can't delete vlanConfig %s because %w"
)

type Validator struct {
	admission.DefaultValidator

	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
	vcCache   ctlnetworkv1.VlanConfigCache
	vsCache   ctlnetworkv1.VlanStatusCache
	vmiCache  ctlkubevirtv1.VirtualMachineInstanceCache
	cnCache   ctlnetworkv1.ClusterNetworkCache
	nodeCache ctlcorev1.NodeCache
}

func NewVlanConfigValidator(
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache,
	vcCache ctlnetworkv1.VlanConfigCache,
	vsCache ctlnetworkv1.VlanStatusCache,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
	cnCache ctlnetworkv1.ClusterNetworkCache,
	nodeCache ctlcorev1.NodeCache,
) *Validator {
	return &Validator{
		nadCache:  nadCache,
		vcCache:   vcCache,
		vsCache:   vsCache,
		vmiCache:  vmiCache,
		cnCache:   cnCache,
		nodeCache: nodeCache,
	}
}

var _ admission.Validator = &Validator{}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	vc := newObj.(*networkv1.VlanConfig)

	if vc.Spec.ClusterNetwork == utils.ManagementClusterNetworkName {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("cluster network can't be %s",
			utils.ManagementClusterNetworkName))
	}

	valid, err := v.isValidNodeSelector(vc.Spec.NodeSelector)
	if err != nil {
		return fmt.Errorf("node selector does not match any existing nodes%v", err)
	}

	if !valid {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("node selector is invalid"))
	}

	if valid, label := utils.IsClusterNetworkLabelValid(vc); !valid {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("cluster network label %s is invalid and must not be updated by user", label))
	}

	if _, err := utils.IsClusterNetworkNameValid(vc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf(createErr, vc.Name, err)
	}

	// check if clusternetwork exists
	if _, err := v.cnCache.Get(vc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("it refers to a none-existing cluster network %s or error %w", vc.Spec.ClusterNetwork, err))
	}

	if err := v.validateMTU(vc); err != nil {
		return fmt.Errorf(createErr, vc.Name, err)
	}

	// note: the mutator has patched the Annotations[utils.KeyMatchedNodes] if selector is set and exclude the witness-node
	nodes, err := getMatchNodes(vc)
	if err != nil {
		return fmt.Errorf(createErr, vc.Name, err)
	}

	if err := v.checkOverlaps(vc, nodes); err != nil {
		return fmt.Errorf(createErr, vc.Name, err)
	}

	return nil
}

func (v *Validator) Update(_ *admission.Request, oldObj, newObj runtime.Object) error {
	oldVc := oldObj.(*networkv1.VlanConfig)
	newVc := newObj.(*networkv1.VlanConfig)

	// ignore the update if the resource is being deleted
	if newVc.DeletionTimestamp != nil {
		return nil
	}

	valid, err := v.isValidNodeSelector(newVc.Spec.NodeSelector)
	if err != nil {
		return err
	}

	if !valid {
		return fmt.Errorf(createErr, newVc.Name, fmt.Errorf("node selector is invalid"))
	}

	if newVc.Spec.ClusterNetwork == utils.ManagementClusterNetworkName {
		return fmt.Errorf(updateErr, newVc.Name, fmt.Errorf("cluster network can't be %s",
			utils.ManagementClusterNetworkName))
	}

	if valid, label := utils.IsClusterNetworkLabelValid(newVc); !valid {
		return fmt.Errorf(updateErr, newVc.Name, fmt.Errorf("cluster network label %s is invalid and must not be updated by user", label))
	}
	// check if clusternetwork exists
	// Harvester UI allows to migration a vlanconfig from one clusternetwork to another
	// but for none-UI, the target ClusterNetwork may be blank
	if _, err := v.cnCache.Get(newVc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf(updateErr, newVc.Name, fmt.Errorf("it refers to a none-existing cluster network %s or error %w", newVc.Spec.ClusterNetwork, err))
	}

	if err := v.validateMTU(newVc); err != nil {
		return fmt.Errorf(updateErr, newVc.Name, err)
	}

	// note: the mutator has patched the Annotations[utils.KeyMatchedNodes] if selector is set and exclude the witness-node
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
	affectedNodes := getAffectedNodes(oldVc, newVc, oldNodes, newNodes)

	// note: the vlanconfig may match no nodes, the affectedNodes can hence be empty
	if err := v.checkVmi(oldVc, affectedNodes); err != nil {
		return fmt.Errorf(updateErr, oldVc.Name, err)
	}

	if err := v.checkStorageNetwork(oldVc, affectedNodes); err != nil {
		return fmt.Errorf(updateErr, oldVc.Name, err)
	}

	return nil
}

func getAffectedNodes(oldVc, newVc *networkv1.VlanConfig, oldNodes, newNodes mapset.Set[string]) mapset.Set[string] {
	// when vlanconfig's MTU/uplink/... is changed, all oldNodes are always affected, all vmis on them should be stopped
	if (oldVc.Spec.ClusterNetwork != newVc.Spec.ClusterNetwork) || !reflect.DeepEqual(oldVc.Spec.Uplink, newVc.Spec.Uplink) {
		return oldNodes
	}

	// if the nodeSelector is changed, or there are more nodes matched, then get the differences
	return oldNodes.Difference(newNodes)
}

func (v *Validator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	vc := oldObj.(*networkv1.VlanConfig)

	nodes, err := getMatchNodes(vc)
	if err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	// note: the vlanconfig may match no nodes
	if err := v.checkVmi(vc, nodes); err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	if err := v.checkStorageNetwork(vc, nodes); err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
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
	if nodes == nil {
		return nil
	}
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

// checkVmi is to confirm if any VMI exists on the affected nodes. Those VMIs must be stopped in advance.
func (v *Validator) checkVmi(vc *networkv1.VlanConfig, nodes mapset.Set[string]) error {
	// note: the vlanconfig's selector may select empty node, e.g. a place-holder vlanconfig
	// when those given nodes are empty, surely no vmi exists on them
	if nodes == nil || nodes.Cardinality() == 0 {
		return nil
	}

	nadGetter := utils.NewNadGetter(v.nadCache)
	nads, err := nadGetter.ListNadsOnClusterNetwork(vc.Spec.ClusterNetwork)
	if err != nil {
		return err
	}

	vmiGetter := utils.NewVmiGetter(v.vmiCache)
	// get vmis on the given nodes
	if vmiStrList, err := vmiGetter.VmiNamesWhoUseNads(nads, true, nodes); err != nil {
		return err
	} else if len(vmiStrList) > 0 {
		return fmt.Errorf("it is blocked by VM(s) %s which must be stopped at first", strings.Join(vmiStrList, ", "))
	}
	return nil
}

// getMatchNodes retrieves the matched nodes from the VlanConfig's annotations
// and returns them as a set.
func getMatchNodes(vc *networkv1.VlanConfig) (mapset.Set[string], error) {
	empty := mapset.NewSet[string]()
	if vc == nil || vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" {
		return empty, nil
	}

	var matchedNodes []string
	if err := json.Unmarshal([]byte(vc.Annotations[utils.KeyMatchedNodes]), &matchedNodes); err != nil {
		return empty, err
	}

	return mapset.NewSet(matchedNodes...), nil
}

func (v *Validator) validateMTU(current *networkv1.VlanConfig) error {
	// MTU can be 0, it means user does not input and the default value is used
	mtu := utils.GetMTUFromVlanConfig(current)
	if !utils.IsValidMTU(mtu) {
		return fmt.Errorf("the MTU %v is out of range [0, %v..%v]", mtu, utils.MinMTU, utils.MaxMTU)
	}

	// ensure all vlanconfigs on one clusternetwork have the same MTU
	vcs, err := v.vcCache.List(labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: current.Spec.ClusterNetwork,
	}).AsSelector())
	if err != nil {
		return err
	}

	for _, vc := range vcs {
		if vc.Name == current.Name || vc.DeletionTimestamp != nil {
			continue
		}
		vcMtu := utils.GetMTUFromVlanConfig(vc)
		if !utils.AreEqualMTUs(mtu, vcMtu) {
			return fmt.Errorf("the vlanconfig %s MTU %v is different with another vlanconfig %s MTU %v, all vlanconfigs on one clusternetwork need to have same MTU", current.Name, mtu, vc.Name, vcMtu)
		}
	}

	return nil
}

// if storagenetwork nad is there, and affected node number > 0, then deny
func (v *Validator) checkStorageNetwork(vc *networkv1.VlanConfig, nodes mapset.Set[string]) error {
	// affect no nodes
	if nodes == nil || nodes.Cardinality() == 0 {
		return nil
	}

	nadGetter := utils.NewNadGetter(v.nadCache)
	nad, err := nadGetter.GetFirstActiveStorageNetworkNadOnClusterNetwork(vc.Spec.ClusterNetwork)
	if err != nil {
		return err
	}

	if nad != nil {
		return fmt.Errorf("the storage network nad %s is still attached", nad.Name)
	}

	return nil
}

func (v *Validator) isValidNodeSelector(nodeSelector map[string]string) (bool, error) {
	if len(nodeSelector) == 0 {
		return true, nil
	}

	selector := labels.SelectorFromSet(nodeSelector)

	nodes, err := v.nodeCache.List(selector)
	if err != nil {
		return false, err
	}

	return len(nodes) > 0, nil
}
