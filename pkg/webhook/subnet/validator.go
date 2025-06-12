package subnet

import (
	"fmt"
	"reflect"
	"strings"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"github.com/harvester/webhook/pkg/server/admission"
	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	kubeovnnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubeovn.io/v1"
)

type Validator struct {
	admission.DefaultValidator
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
	vpcCache kubeovnnetworkv1.VpcCache
}

var _ admission.Validator = &Validator{}

func NewSubnetValidator(nadCache ctlcniv1.NetworkAttachmentDefinitionCache, vpcCache kubeovnnetworkv1.VpcCache) *Validator {
	return &Validator{
		nadCache: nadCache,
		vpcCache: vpcCache,
	}
}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	subnet := newObj.(*kubeovnv1.Subnet)

	subnetSpec := subnet.Spec

	if subnetSpec.Provider == "" {
		return fmt.Errorf("provider is empty for subnet %s", subnet.Name)
	}

	err := v.checkIfNadExists(subnetSpec.Provider)
	if err != nil {
		return fmt.Errorf("nad does not exist for subnet %s err=%v", subnet.Name, err)
	}

	err = v.checkIfVpcExists(subnetSpec.Vpc)
	if err != nil {
		return fmt.Errorf("vpc does not exist for subnet %s err=%v", subnet.Name, err)
	}

	return nil
}

func (v *Validator) Update(_ *admission.Request, oldObj, newObj runtime.Object) error {
	newSubnet := newObj.(*kubeovnv1.Subnet)
	oldSubnet := oldObj.(*kubeovnv1.Subnet)

	// ignore the update if the resource is being deleted
	if newSubnet.DeletionTimestamp != nil {
		return nil
	}

	newSubnetSpec := newSubnet.Spec
	oldSubnetSpec := oldSubnet.Spec

	// skip the update if the config is not changed
	if reflect.DeepEqual(newSubnetSpec, oldSubnetSpec) {
		return nil
	}

	if newSubnetSpec.Provider == "" {
		return fmt.Errorf("provider is empty for subnet %s", newSubnet.Name)
	}

	err := v.checkIfNadExists(newSubnetSpec.Provider)
	if err != nil {
		return fmt.Errorf("nad does not exist for subnet %s err=%v", newSubnet.Name, err)
	}

	err = v.checkIfVpcExists(newSubnetSpec.Vpc)
	if err != nil {
		return fmt.Errorf("vpc does not exist for subnet %s err=%v", newSubnet.Name, err)
	}

	return nil
}

func (v *Validator) checkIfNadExists(subnetProvider string) (err error) {
	providerName := strings.Split(subnetProvider, ".")
	if len(providerName) < 2 {
		return fmt.Errorf("invalid provider for subnet %s", subnetProvider)
	}

	_, err = v.nadCache.Get(providerName[1], providerName[0])
	if err != nil && apierrors.IsNotFound(err) {
		return fmt.Errorf("nad %s not created in namespace %s, create the nad before subnet creation", providerName[0], providerName[1])
	}

	return err
}

func (v *Validator) checkIfVpcExists(subnetVpc string) (err error) {
	if subnetVpc != "" {
		_, err = v.vpcCache.Get(subnetVpc)
		if err != nil && apierrors.IsNotFound(err) {
			return fmt.Errorf("vpc %s not created for subnet, create vpc before subnet creation", subnetVpc)
		}
	}

	return err
}

func (v *Validator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"subnets"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   kubeovnv1.SchemeGroupVersion.Group,
		APIVersion: kubeovnv1.SchemeGroupVersion.Version,
		ObjectType: &kubeovnv1.Subnet{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}
