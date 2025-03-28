package vpc

import (
	"fmt"
	"net"
	"reflect"

	"github.com/harvester/webhook/pkg/server/admission"
	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	kubeovnnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubeovn.io/v1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type Validator struct {
	admission.DefaultValidator
	subnetCache kubeovnnetworkv1.SubnetCache
}

var _ admission.Validator = &Validator{}

func NewVpcValidator(subnetCache kubeovnnetworkv1.SubnetCache) *Validator {
	return &Validator{
		subnetCache: subnetCache,
	}
}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	vpc := newObj.(*kubeovnv1.Vpc)

	err := v.checkVpcConfig(*vpc)
	if err != nil {
		return fmt.Errorf("failed to validate vpc spec config for vpc %s in %s err=%v", vpc.Name, vpc.Namespace, err)
	}

	return nil
}

func (v *Validator) Update(_ *admission.Request, oldObj, newObj runtime.Object) error {
	newVpc := newObj.(*kubeovnv1.Vpc)
	oldVpc := oldObj.(*kubeovnv1.Vpc)

	// ignore the update if the resource is being deleted
	if newVpc.DeletionTimestamp != nil {
		return nil
	}

	newVpcSpec := newVpc.Spec
	oldVpcSpec := oldVpc.Spec

	// skip the update if the config is not changed
	if reflect.DeepEqual(newVpcSpec, oldVpcSpec) {
		return nil
	}

	err := v.checkVpcConfig(*newVpc)
	if err != nil {
		return fmt.Errorf("invalid vpc spec config for vpc %s in %s err=%v", newVpc.Name, newVpc.Namespace, err)
	}

	return nil
}

func (v *Validator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	vpc := oldObj.(*kubeovnv1.Vpc)

	//check for subnets using this VPC
	subnets, err := v.subnetCache.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list subnets for vpc %s in %s", vpc.Name, vpc.Namespace)
	}

	for _, subnet := range subnets {
		if subnet.Spec.Vpc == vpc.Name {
			return fmt.Errorf("delete the subnet %s associated first before deleting vpc %s in %s", subnet.Name, vpc.Name, vpc.Namespace)
		}
	}

	return nil
}

func (v *Validator) checkVpcConfig(vpcConfig kubeovnv1.Vpc) error {
	vpcConfigSpec := vpcConfig.Spec

	staticRoutes := vpcConfigSpec.StaticRoutes
	for _, staticRoute := range staticRoutes {
		_, ipnet, err := net.ParseCIDR(staticRoute.CIDR)
		if err != nil || (ipnet != nil && utils.IsMaskZero(ipnet)) {
			return fmt.Errorf("the CIDR %s is invalid", staticRoute.CIDR)
		}

		if net.ParseIP(staticRoute.NextHopIP) == nil {
			return fmt.Errorf("the NextHop %s is invalid", staticRoute.NextHopIP)
		}
	}

	vpcPeerings := vpcConfigSpec.VpcPeerings
	for _, vpcPeering := range vpcPeerings {
		if net.ParseIP(vpcPeering.LocalConnectIP) == nil {
			return fmt.Errorf("the LocalConnectIP %s is invalid", vpcPeering.LocalConnectIP)
		}
	}

	return nil
}

func (v *Validator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"vpcs"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   kubeovnv1.SchemeGroupVersion.Group,
		APIVersion: kubeovnv1.SchemeGroupVersion.Version,
		ObjectType: &kubeovnv1.Vpc{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}
