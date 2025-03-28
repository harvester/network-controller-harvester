package subnet

import (
	"fmt"
	"net"
	"reflect"
	"strings"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"github.com/harvester/webhook/pkg/server/admission"
	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	kubeovnnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubeovn.io/v1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
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
		return fmt.Errorf("provider is empty for subnet %s in %s", subnet.Name, subnet.Namespace)
	}

	if subnetSpec.CIDRBlock == "" || subnetSpec.Gateway == "" {
		return fmt.Errorf("subnet cidr/gw is empty for subnet %s in %s", subnet.Name, subnet.Namespace)
	}

	err := v.checkCIDRGW(subnetSpec.CIDRBlock, subnetSpec.Gateway)
	if err != nil {
		return fmt.Errorf("invalid cidr/gw for subnet %s in %s err=%v", subnet.Name, subnet.Namespace, err)
	}

	err = v.checkifNadExists(subnetSpec.Provider)
	if err != nil {
		return fmt.Errorf("nad does not exists for subnet %s in %s err=%v", subnet.Name, subnet.Namespace, err)
	}

	err = v.checkifVpcExists(subnetSpec.Vpc)
	if err != nil {
		return fmt.Errorf("vpc does not exists for subnet %s in %s err=%v", subnet.Name, subnet.Namespace, err)
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
		return fmt.Errorf("provider is empty for subnet %s in %s", newSubnet.Name, newSubnet.Namespace)
	}

	err := v.checkCIDRGW(newSubnetSpec.CIDRBlock, newSubnetSpec.Gateway)
	if err != nil {
		return fmt.Errorf("invalid cidr/gw for subnet %s in %s err=%v", newSubnet.Name, newSubnet.Namespace, err)
	}

	err = v.checkifNadExists(newSubnetSpec.Provider)
	if err != nil {
		return fmt.Errorf("nad does not exists for subnet %s in %s err=%v", newSubnet.Name, newSubnet.Namespace, err)
	}

	err = v.checkifVpcExists(newSubnetSpec.Vpc)
	if err != nil {
		return fmt.Errorf("vpc does not exists for subnet %s in %s err=%v", newSubnet.Name, newSubnet.Namespace, err)
	}

	return nil
}

func (v *Validator) checkifNadExists(subnetProvider string) error {
	providerName := strings.Split(subnetProvider, ".")
	_, err := v.nadCache.Get(providerName[1], providerName[0])
	if err != nil {
		return fmt.Errorf("failed to get nad %s %s created for the subnet", providerName[1], providerName[0])
	}

	return nil
}

func (v *Validator) checkifVpcExists(subnetVpc string) error {
	if subnetVpc != "" {
		_, err := v.vpcCache.Get(subnetVpc)
		if err != nil {
			return fmt.Errorf("vpc %s not created for subnet", subnetVpc)
		}
	}

	return nil
}

func (v *Validator) checkCIDRGW(cidr string, gw string) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil || (ipnet != nil && utils.IsMaskZero(ipnet)) {
		return fmt.Errorf("the CIDR %s is invalid", cidr)
	}

	if net.ParseIP(gw) == nil {
		return fmt.Errorf("the gateway %s is invalid", gw)
	}

	return nil
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
