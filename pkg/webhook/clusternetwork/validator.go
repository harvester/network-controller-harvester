package clusternetwork

import (
	"fmt"
	"strings"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/webhook/pkg/server/admission"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	createErr = "can't create cluster network %s because %w"
	updateErr = "can't update cluster network %s because %w"
	deleteErr = "can't delete cluster network %s because %w"

	maxClusterNetworkNameLen = iface.MaxDeviceNameLen - len(iface.BridgeSuffix)
)

type CnValidator struct {
	admission.DefaultValidator

	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache
	vcCache  ctlnetworkv1.VlanConfigCache
}

var _ admission.Validator = &CnValidator{}

func NewCnValidator(nadCache ctlcniv1.NetworkAttachmentDefinitionCache, vmiCache ctlkubevirtv1.VirtualMachineInstanceCache, vcCache ctlnetworkv1.VlanConfigCache) *CnValidator {
	validator := &CnValidator{
		nadCache: nadCache,
		vmiCache: vmiCache,
		vcCache:  vcCache,
	}
	return validator
}

func (c *CnValidator) Create(_ *admission.Request, newObj runtime.Object) error {
	cn := newObj.(*networkv1.ClusterNetwork)

	if len(cn.Name) > maxClusterNetworkNameLen {
		return fmt.Errorf(createErr, cn.Name, fmt.Errorf("the length of name is more than %d", maxClusterNetworkNameLen))
	}

	if err := checkMTUOfNewClusterNetwork(cn); err != nil {
		return fmt.Errorf(createErr, cn.Name, err)
	}

	return nil
}

func (c *CnValidator) Update(_ *admission.Request, oldObj, newObj runtime.Object) error {
	oldCn := oldObj.(*networkv1.ClusterNetwork)
	newCn := newObj.(*networkv1.ClusterNetwork)

	if err := c.checkMTUOfUpdatedClusterNetwork(oldCn, newCn); err != nil {
		return fmt.Errorf(updateErr, newCn.Name, err)
	}

	if err := c.checkMTUOfUpdatedMgmtClusterNetwork(oldCn, newCn); err != nil {
		return fmt.Errorf(updateErr, newCn.Name, err)
	}

	return nil
}

func (c *CnValidator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	cn := oldObj.(*networkv1.ClusterNetwork)

	if cn.Name == utils.ManagementClusterNetworkName {
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("it is not allowed"))
	}

	// all related vcs should be deleted/migrated to others
	vcs, err := c.vcCache.List(labels.Set{
		utils.KeyClusterNetworkLabel: cn.Name,
	}.AsSelector())
	if err != nil {
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("failed to list vlanconfig error %w", err))
	}

	if len(vcs) > 0 {
		vcNameList := make([]string, 0, len(vcs))
		for _, vc := range vcs {
			vcNameList = append(vcNameList, vc.Name)
		}
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("vlanconfig(s) %v under this clusternetwork are still existing", vcNameList))
	}

	// all related nads should be deleted
	nadGetter := utils.NewNadGetter(c.nadCache)
	nadStrList, err := nadGetter.NadNamesOnClusterNetwork(cn.Name)
	if err != nil {
		return fmt.Errorf(deleteErr, cn.Name, err)
	}

	if len(nadStrList) > 0 {
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("nads(s) %v under this clusternetwork are still existing", strings.Join(nadStrList, ", ")))
	}

	// no need to check vmi, vm, both of them need to be stopped &/ removed related interfaces/networks when deleting nads
	return nil
}

func (c *CnValidator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"clusternetworks"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   networkv1.SchemeGroupVersion.Group,
		APIVersion: networkv1.SchemeGroupVersion.Version,
		ObjectType: &networkv1.ClusterNetwork{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

func checkMTUOfNewClusterNetwork(cn *networkv1.ClusterNetwork) error {
	if cn == nil || cn.Annotations == nil {
		return nil
	}

	// for non-mgmt cluster network, this annotation can only be operated by controller
	if _, ok := cn.Annotations[utils.KeyUplinkMTU]; ok {
		return fmt.Errorf("annotation %v can't be added", utils.KeyUplinkMTU)
	}
	return nil
}

// for non-mgmt cluster network
func (c *CnValidator) checkMTUOfUpdatedClusterNetwork(oldCn, newCn *networkv1.ClusterNetwork) error {
	if oldCn == nil || newCn == nil || newCn.Name == utils.ManagementClusterNetworkName {
		return nil
	}

	newMtu := utils.DefaultMTU
	var err error
	if mtuStr, ok := newCn.Annotations[utils.KeyUplinkMTU]; ok {
		if newMtu, err = utils.GetMTUFromAnnotation(mtuStr); err != nil {
			return err
		}
	}

	// ensure clusternetwork's MTU is same with all vlanconfigs
	vcs, err := c.vcCache.List(labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: newCn.Name,
	}).AsSelector())
	if err != nil {
		return err
	}

	for _, vc := range vcs {
		vcMtu := utils.GetMTUFromVlanConfig(vc)
		if !utils.AreEqualMTUs(vcMtu, newMtu) {
			return fmt.Errorf("clusternetwork has MTU %v, but the vlanconfigs %v has another MTU %v", newMtu, vc.Name, vcMtu)
		}
	}

	return nil
}

// mgmt cluster network, there is no vlanconfig to configure MTU, the MTU is configured in node installation stage and saved to local file
// later we need to convert each node's network configuration to a related vlanconfig object
// currently, if user plans to set a none-default MTU value, then it can be updated via clusternetwork mgmt annotation
func (c *CnValidator) checkMTUOfUpdatedMgmtClusterNetwork(oldCn, newCn *networkv1.ClusterNetwork) error {
	if oldCn == nil || newCn == nil || newCn.Name != utils.ManagementClusterNetworkName {
		return nil
	}

	// mgmt network, MTU can be updated
	newMtu := utils.DefaultMTU
	var err error
	if mtuStr, ok := newCn.Annotations[utils.KeyUplinkMTU]; ok {
		if newMtu, err = utils.GetMTUFromAnnotation(mtuStr); err != nil {
			return err
		}
	}

	oldMtu := utils.DefaultMTU
	if mtuStr, ok := oldCn.Annotations[utils.KeyUplinkMTU]; ok {
		if oldMtu, err = utils.GetMTUFromAnnotation(mtuStr); err != nil {
			return err
		}
	}

	// MTU does not change
	if utils.AreEqualMTUs(oldMtu, newMtu) {
		return nil
	}

	// for mgmt network, the nad is not tied to any vlanconfig, check nad directly
	nadGetter := utils.NewNadGetter(c.nadCache)
	nads, err := nadGetter.ListNadsOnClusterNetwork(newCn.Name)
	if err != nil {
		return err
	}

	if nad := utils.FilterFirstActiveStorageNetworkNad(nads); nad != nil {
		return fmt.Errorf("the MTU can't be changed from %v to %v as storage network nad %s is still attached", oldMtu, newMtu, nad.Name)
	}

	vmiGetter := utils.NewVmiGetter(c.vmiCache)
	// get all, no filter
	if vmiStrList, err := vmiGetter.VmiNamesWhoUseNads(nads, false, nil); err != nil {
		return err
	} else if len(vmiStrList) > 0 {
		return fmt.Errorf("the MTU can't be changed from %v to %v as following VMs must be stopped at first: %s", oldMtu, newMtu, strings.Join(vmiStrList, ", "))
	}

	return nil
}
