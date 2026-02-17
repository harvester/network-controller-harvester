package subnet

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/harvester/webhook/pkg/server/admission"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	kubeovnnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubeovn.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type Validator struct {
	admission.DefaultValidator
	nadCache    ctlcniv1.NetworkAttachmentDefinitionCache
	subnetCache kubeovnnetworkv1.SubnetCache
	vpcCache    kubeovnnetworkv1.VpcCache
	vmiCache    ctlkubevirtv1.VirtualMachineInstanceCache
}

var _ admission.Validator = &Validator{}

func NewSubnetValidator(nadCache ctlcniv1.NetworkAttachmentDefinitionCache, subnetCache kubeovnnetworkv1.SubnetCache, vpcCache kubeovnnetworkv1.VpcCache, vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) *Validator {
	return &Validator{
		nadCache:    nadCache,
		subnetCache: subnetCache,
		vpcCache:    vpcCache,
		vmiCache:    vmiCache,
	}
}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	subnet := newObj.(*kubeovnv1.Subnet)

	subnetSpec := subnet.Spec

	isReserved, err := utils.IsReservedSubnet(subnet.Name, subnetSpec.Provider)
	if isReserved {
		return err
	}

	err = v.checkIfValidNad(subnetSpec.Provider)
	if err != nil {
		return err
	}

	err = v.checkSubnetsUsingNAD(subnetSpec.Provider)
	if err != nil {
		return err
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

	isReserved, err := utils.IsReservedSubnet(newSubnet.Name, newSubnetSpec.Provider)
	if isReserved {
		return err
	}

	if newSubnetSpec.Provider != oldSubnetSpec.Provider && oldSubnet.Status.V4UsingIPs != 0 {
		return fmt.Errorf("cannot update provider %s for subnet %s as VMs are still using it", newSubnetSpec.Provider, oldSubnet.Name)
	}

	if newSubnetSpec.Vpc != oldSubnetSpec.Vpc && oldSubnet.Status.V4UsingIPs != 0 {
		return fmt.Errorf("cannot update vpc %s for subnet %s as VMs are still using it", newSubnetSpec.Vpc, oldSubnet.Name)
	}

	err = v.checkIfValidNad(newSubnetSpec.Provider)
	if err != nil {
		return err
	}

	if newSubnetSpec.Provider != oldSubnetSpec.Provider {
		err = v.checkSubnetsUsingNAD(newSubnetSpec.Provider)
		if err != nil {
			return err
		}
	}

	err = v.checkIfVpcExists(newSubnetSpec.Vpc)
	if err != nil {
		return fmt.Errorf("vpc does not exist for subnet %s err=%v", newSubnet.Name, err)
	}

	//DHCP setting update is allowed when there is no VM using the subnet, otherwise the VMs must be stopped before update
	//This will make sure to update the interface binding method on the VM from managedtap(dhcp enabled) to bridge(dhcp disabled) or vice versa.
	err = v.canUpdateDHCPSetting(oldSubnet, newSubnet)
	if err != nil {
		return err
	}

	return nil
}

func (v *Validator) canUpdateDHCPSetting(oldSubnet, newSubnet *kubeovnv1.Subnet) error {
	if oldSubnet.Spec.EnableDHCP == newSubnet.Spec.EnableDHCP {
		return nil
	}

	nadName, nadNamespace, err := utils.GetNadNameFromProvider(newSubnet.Spec.Provider)
	if err != nil {
		return err
	}

	subnetNad, err := v.nadCache.Get(nadNamespace, nadName)
	if err != nil {
		return fmt.Errorf("failed to get nad %s in namespace %s for subnet %s err=%v", nadName, nadNamespace, newSubnet.Name, err)
	}
	if err := v.checkVmi(subnetNad); err != nil {
		return fmt.Errorf("cannot update DHCP setting for subnet %s as VMs are still using it, stop the VMs and update err:%w", newSubnet.Name, err)
	}

	return nil
}

func (v *Validator) checkVmi(nad *cniv1.NetworkAttachmentDefinition) error {
	vmiGetter := utils.NewVmiGetter(v.vmiCache)
	// get all, no filter
	if vmiStrList, err := vmiGetter.VmiNamesWhoUseNad(nad, false, nil); err != nil {
		return err
	} else if len(vmiStrList) > 0 {
		return fmt.Errorf("it's still used by VM(s) %s which must be stopped at first", strings.Join(vmiStrList, ", "))
	}

	return nil
}

func (v *Validator) checkIfValidNad(subnetProvider string) (err error) {
	nadName, nadNamespace, err := utils.GetNadNameFromProvider(subnetProvider)
	if err != nil {
		return err
	}

	nad, err := v.nadCache.Get(nadNamespace, nadName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("nad %s not created in namespace %s, create the nad before subnet creation", nadName, nadNamespace)
		}
		return err
	}

	if nad.Labels == nil {
		return fmt.Errorf("lables are empty for nad %s/%s", nadNamespace, nadName)
	}

	if nad.Labels[utils.KeyNetworkType] != string(utils.OverlayNetwork) {
		return fmt.Errorf("network type of nad is not kubeovn instead %s", nad.Labels[utils.KeyNetworkType])
	}

	return nil
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

func (v *Validator) checkSubnetsUsingNAD(provider string) error {
	subnets, err := v.subnetCache.List(k8slabels.Everything())
	if err != nil {
		return fmt.Errorf("failed to retrieve subnets err=%v", err)
	}

	//nadname.nadnamespace.ovn is 1:1 mapping with a subnet
	//same nad name in a different namespace can be attached to different subnets
	for _, subnet := range subnets {
		if subnet.Spec.Provider == provider {
			return fmt.Errorf("subnet %s is using the provider %s already", subnet.Name, provider)
		}
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
