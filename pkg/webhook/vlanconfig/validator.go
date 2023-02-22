package vlanconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/webhook/pkg/types"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

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

type Validator struct {
	types.DefaultValidator

	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
	vsCache  ctlnetworkv1.VlanStatusCache
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache
}

func NewVlanConfigValidator(
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache,
	vsCache ctlnetworkv1.VlanStatusCache,
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) *Validator {
	return &Validator{
		nadCache: nadCache,
		vsCache:  vsCache,
		vmiCache: vmiCache,
	}
}

var _ types.Validator = &Validator{}

func (v *Validator) Create(_ *types.Request, newObj runtime.Object) error {
	vc := newObj.(*networkv1.VlanConfig)

	if vc.Spec.ClusterNetwork == utils.ManagementClusterNetworkName {
		return fmt.Errorf(createErr, vc.Name, fmt.Errorf("cluster network could not be %s",
			utils.ManagementClusterNetworkName))
	}

	nodes, err := getMatchNodesMap(vc)
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

func (v *Validator) Update(_ *types.Request, oldObj, newObj runtime.Object) error {
	oldVc := oldObj.(*networkv1.VlanConfig)
	newVc := newObj.(*networkv1.VlanConfig)

	if newVc.Spec.ClusterNetwork == utils.ManagementClusterNetworkName {
		return fmt.Errorf(updateErr, newVc.Name, fmt.Errorf("cluster network could not be %s",
			utils.ManagementClusterNetworkName))
	}

	newNodes, err := getMatchNodesMap(newVc)
	if err != nil {
		return fmt.Errorf(updateErr, newVc.Name, err)
	}

	if err := v.checkOverlaps(newVc, newNodes); err != nil {
		return fmt.Errorf(updateErr, newVc.Name, err)
	}

	oldNodes, err := getMatchNodesMap(oldVc)
	if err != nil {
		return fmt.Errorf(updateErr, oldVc.Name, err)
	}

	// get affected nodes after updating
	affectedNodes := getAffectedNodesMap(oldVc.Spec.ClusterNetwork, newVc.Spec.ClusterNetwork, oldNodes, newNodes)
	if err := v.checkVmi(oldVc, affectedNodes); err != nil {
		return fmt.Errorf(updateErr, oldVc.Name, err)
	}

	return nil
}

func getAffectedNodesMap(oldCn, newCn string, oldNodesMap, newNodesMap map[string]bool) map[string]bool {
	affectedNodesMap := make(map[string]bool, len(oldNodesMap))
	if newCn != oldCn {
		affectedNodesMap = oldNodesMap
	} else {
		for n := range oldNodesMap {
			if !newNodesMap[n] {
				affectedNodesMap[n] = true
			}
		}
	}

	return affectedNodesMap
}

func (v *Validator) Delete(_ *types.Request, oldObj runtime.Object) error {
	vc := oldObj.(*networkv1.VlanConfig)

	nodes, err := getMatchNodesMap(vc)
	if err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	if err := v.checkVmi(vc, nodes); err != nil {
		return fmt.Errorf(deleteErr, vc.Name, err)
	}

	return nil
}

func (v *Validator) Resource() types.Resource {
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

func (v *Validator) checkOverlaps(vc *networkv1.VlanConfig, nodesMap map[string]bool) error {
	overlapNods := make([]string, 0, len(nodesMap))
	for node := range nodesMap {
		vsName := utils.Name("", vc.Spec.ClusterNetwork, node)
		if vs, err := v.vsCache.Get(vsName); err != nil && !apierrors.IsNotFound(err) {
			return err
		} else if err == nil && vs.Status.VlanConfig != vc.Name {
			// The vlanconfig is found means a vlanconfig with the same cluster network has been taken effect on this node.
			overlapNods = append(overlapNods, node)
		}
	}

	if len(overlapNods) > 0 {
		return fmt.Errorf("it overlaps with other vlanconfigs matching node(s) %v", overlapNods)
	}

	return nil
}

// checkVmi is to confirm if any VMIs will be affected on affected nodes. Those VMIs must be stopped in advance.
func (v *Validator) checkVmi(vc *networkv1.VlanConfig, nodesMap map[string]bool) error {
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
		vmisTemp, err := vmiGetter.WhoUseNad(nad, nodesMap)
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

func getMatchNodesMap(vc *networkv1.VlanConfig) (map[string]bool, error) {
	if vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" {
		return nil, nil
	}

	var matchedNodes []string
	if err := json.Unmarshal([]byte(vc.Annotations[utils.KeyMatchedNodes]), &matchedNodes); err != nil {
		return nil, err
	}

	nodesMap := make(map[string]bool)
	for _, node := range matchedNodes {
		nodesMap[node] = true
	}

	return nodesMap, nil
}
