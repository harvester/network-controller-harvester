package clusternetwork

import (
	"fmt"

	"github.com/harvester/webhook/pkg/types"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	deleteErr = "could not delete cluster network %s because %w"
)

type CnValidator struct {
	types.DefaultValidator
	vcCache ctlnetworkv1.VlanConfigCache
}

var _ types.Validator = &CnValidator{}

func NewCnValidator(vcCache ctlnetworkv1.VlanConfigCache) *CnValidator {
	validator := &CnValidator{
		vcCache: vcCache,
	}
	return validator
}

func (c *CnValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	cn := oldObj.(*networkv1.ClusterNetwork)

	if cn.Name == utils.ManagementClusterNetworkName {
		return fmt.Errorf(deleteErr, cn.Name, fmt.Errorf("it's not allowed"))
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

func (c *CnValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"clusternetworks"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   networkv1.SchemeGroupVersion.Group,
		APIVersion: networkv1.SchemeGroupVersion.Version,
		ObjectType: &networkv1.ClusterNetwork{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Delete,
		},
	}
}
