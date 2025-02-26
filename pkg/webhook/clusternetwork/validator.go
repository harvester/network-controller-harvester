package clusternetwork

import (
	"fmt"
	"strings"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/webhook/pkg/server/admission"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
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

	// mgmt cluster network is ensured and created by controller
	if cn.Name == utils.ManagementClusterNetworkName {
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("it is not allowed"))
	}

	if len(cn.Name) > maxClusterNetworkNameLen {
		return fmt.Errorf(createErr, cn.Name, fmt.Errorf("the length of name is more than %d", maxClusterNetworkNameLen))
	}

	if err := checkMTUOfNewClusterNetwork(cn); err != nil {
		return err
	}

	return nil
}

func (c *CnValidator) Update(_ *admission.Request, oldObj, newObj runtime.Object) error {
	oldCn := oldObj.(*networkv1.ClusterNetwork)
	newCn := newObj.(*networkv1.ClusterNetwork)

	if err := c.checkMTUOfUpdatedClusterNetwork(oldCn, newCn); err != nil {
		return err
	}

	if err := c.checkMTUOfUpdatedMgmtClusterNetwork(oldCn, newCn); err != nil {
		return err
	}

	return nil
}

func (c *CnValidator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	cn := oldObj.(*networkv1.ClusterNetwork)

	if cn.Name == utils.ManagementClusterNetworkName {
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("it is not allowed"))
	}

	// all related vcs should be deleted
	vcs, err := c.vcCache.List(labels.Set{
		utils.KeyClusterNetworkLabel: cn.Name,
	}.AsSelector())
	if err != nil {
		return err
	}

	if len(vcs) > 0 {
		vcNameList := make([]string, 0, len(vcs))
		for _, vc := range vcs {
			vcNameList = append(vcNameList, vc.Name)
		}
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("vlanconfig(s) %v under this clusternetwork are still existing", vcNameList))
	}

	// all related nads should be deleted
	nads, err := c.nadCache.List(corev1.NamespaceAll, labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: cn.Name,
	}).AsSelector())
	if err != nil {
		return fmt.Errorf(deleteErr, cn.Name, err)
	}

	if len(nads) == 0 {
		return nil
	}

	nadStrList := make([]string, len(nads))
	for i, nad := range nads {
		nadStrList[i] = nad.Namespace + "/" + nad.Name
	}

	return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("nads(s) %v under this clusternetwork are still existing", strings.Join(nadStrList, ", ")))

	// TODO: check vmi, vm as well?
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
	if cn == nil || cn.Labels == nil {
		return nil
	}

	// for none-mgmt cluster network, this label can only be operated by controller
	if _, ok := cn.Labels[utils.KeyUplinkMTU]; ok {
		return fmt.Errorf(createErr, cn.Name, fmt.Errorf("label %v can't be added", utils.KeyUplinkMTU))
	}
	return nil
}

// for none-mgmt cluster network
func (c *CnValidator) checkMTUOfUpdatedClusterNetwork(oldCn, newCn *networkv1.ClusterNetwork) error {
	if oldCn == nil || newCn == nil || newCn.Name == utils.ManagementClusterNetworkName {
		return nil
	}

	// user can't add or update this label, but can delete it
	oldMtu, _ := oldCn.Labels[utils.KeyUplinkMTU]
	newMtu, ok := newCn.Labels[utils.KeyUplinkMTU]

	// deleted, or none-existing
	if !ok {
		return nil
	}

	if newMtu != oldMtu {
		return fmt.Errorf(updateErr, newCn.Name, fmt.Errorf("label %v can't be added/changed from %v to %v", utils.KeyUplinkMTU, oldMtu, newMtu))
	}

	return nil
}

// mgmt cluster network, there is no vlanconfig to configure MTU, the MTU is configured in node installation stage and saved to local file
// later we need to convert each node's network configuration to a related vlanconfig object
// currently, if user plans to set a none-default MTU value, then it can be updated via clusternetwork mgmt label/annotation
func (c *CnValidator) checkMTUOfUpdatedMgmtClusterNetwork(oldCn, newCn *networkv1.ClusterNetwork) error {
	if oldCn == nil || newCn == nil || newCn.Name != utils.ManagementClusterNetworkName {
		return nil
	}

	// mgmt network, MTU can be updated
	newMtu := utils.DefaultMTU
	var err error
	if mtu, ok := newCn.Labels[utils.KeyUplinkMTU]; ok {
		if newMtu, err = utils.GetMTUFromLabel(mtu); err != nil {
			return fmt.Errorf(updateErr, newCn.Name, err)
		}
	}

	oldMtu := utils.DefaultMTU
	if mtu, ok := oldCn.Labels[utils.KeyUplinkMTU]; ok {
		if oldMtu, err = utils.GetMTUFromLabel(mtu); err != nil {
			return fmt.Errorf(updateErr, oldCn.Name, err)
		}
	}

	// MTU does not change
	if utils.AreEqualMTUs(oldMtu, newMtu) {
		return nil
	}

	// for mgmt network, the nad is not tied to any vlanconfig, check nad directly
	nads, err := c.nadCache.List(corev1.NamespaceAll, labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: newCn.Name,
	}).AsSelector())
	if err != nil {
		return fmt.Errorf(updateErr, newCn.Name, err)
	}

	if len(nads) == 0 {
		return nil
	}

	for _, nad := range nads {
		if utils.IsStorageNetworkNad(nad) && nad.DeletionTimestamp == nil {
			return fmt.Errorf(updateErr, newCn.Name, fmt.Errorf("the MTU can't be changed from %v to %v as storage network nad %s is still attached", oldMtu, newMtu, nad.Name))
		}
	}

	vmiGetter := utils.VmiGetter{VmiCache: c.vmiCache}
	if vmiStrList, err := vmiGetter.VmiNameWhoUseNads(nads, nil); err != nil {
		return err
	} else if len(vmiStrList) > 0 {
		return fmt.Errorf(updateErr, newCn.Name, fmt.Errorf("the MTU can't be changed from %v to %v as following VMs must be stopped at first: %s", oldMtu, newMtu, strings.Join(vmiStrList, ", ")))
	}

	return nil
}
