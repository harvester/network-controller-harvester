package clusternetwork

import (
	"fmt"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
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
	createErr                           = "could not create cluster network %s because %w"
	deleteErr                           = "could not delete cluster network %s because %w"
	StorageNetworkNetAttachDefNamespace = "harvester-system"
)

type CnValidator struct {
	admission.DefaultValidator
	vcCache  ctlnetworkv1.VlanConfigCache
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
}

var _ admission.Validator = &CnValidator{}

func NewCnValidator(vcCache ctlnetworkv1.VlanConfigCache, nadCache ctlcniv1.NetworkAttachmentDefinitionCache) *CnValidator {
	validator := &CnValidator{
		vcCache:  vcCache,
		nadCache: nadCache,
	}
	return validator
}

func (c *CnValidator) Create(_ *admission.Request, newObj runtime.Object) error {
	cn := newObj.(*networkv1.ClusterNetwork)

	maxClusterNetworkNameLen := iface.MaxDeviceNameLen - len(iface.BridgeSuffix)

	if len(cn.Name) > maxClusterNetworkNameLen {
		return fmt.Errorf(createErr, cn.Name, fmt.Errorf("the length of the clusterNetwork value is "+
			"more than %d", maxClusterNetworkNameLen))
	}

	return nil
}

func (c *CnValidator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	cn := oldObj.(*networkv1.ClusterNetwork)

	if cn.Name == utils.ManagementClusterNetworkName {
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("it's not allowed"))
	}

	nads, err := c.nadCache.List(StorageNetworkNetAttachDefNamespace, labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: cn.Name,
	}).AsSelector())
	if err != nil {
		return fmt.Errorf(deleteErr, cn.Name, err)
	}

	if len(nads) > 0 {
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("storage network is still attached"))
	}

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
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("vlanconfig(s) %v under this clusternetwork is/are "+
			"still exist(s)", vcNameList))
	}

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
			admissionregv1.Delete,
		},
	}
}
