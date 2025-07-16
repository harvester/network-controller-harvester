package nad

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/harvester/webhook/pkg/server/admission"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"

	kubeovnnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubeovn.io/v1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	createErr = "can't create nad %s/%s because %w"
	updateErr = "can't update nad %s/%s because %w"
	deleteErr = "can't delete nad %s/%s because %w"

	storageNetworkErr = "it is used by storagenetwork"
)

type Validator struct {
	admission.DefaultValidator
	vmCache       ctlkubevirtv1.VirtualMachineCache
	vmiCache      ctlkubevirtv1.VirtualMachineInstanceCache
	cnCache       ctlnetworkv1.ClusterNetworkCache
	vcCache       ctlnetworkv1.VlanConfigCache
	subnetCache   kubeovnnetworkv1.SubnetCache
	subnetEnabled bool
}

var _ admission.Validator = &Validator{}

func NewNadValidator(vmCache ctlkubevirtv1.VirtualMachineCache, vmiCache ctlkubevirtv1.VirtualMachineInstanceCache, cnCache ctlnetworkv1.ClusterNetworkCache, vcCache ctlnetworkv1.VlanConfigCache, subnetCache kubeovnnetworkv1.SubnetCache, subnetEnabled bool) *Validator {
	return &Validator{
		vmCache:       vmCache,
		vmiCache:      vmiCache,
		cnCache:       cnCache,
		vcCache:       vcCache,
		subnetCache:   subnetCache,
		subnetEnabled: subnetEnabled,
	}
}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	nad := newObj.(*cniv1.NetworkAttachmentDefinition)

	if err := v.checkRoute(nad.Annotations[utils.KeyNetworkRoute]); err != nil {
		return fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}

	conf, err := decodeConfig(nad.Spec.Config)
	if err != nil {
		return fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}

	if err := v.checkNadConfig(conf, nad); err != nil {
		return fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}

	return nil
}

func (v *Validator) Update(_ *admission.Request, oldObj, newObj runtime.Object) error {
	newNad := newObj.(*cniv1.NetworkAttachmentDefinition)
	oldNad := oldObj.(*cniv1.NetworkAttachmentDefinition)

	// ignore the update if the resource is being deleted
	if newNad.DeletionTimestamp != nil {
		return nil
	}

	if err := v.checkRoute(newNad.Annotations[utils.KeyNetworkRoute]); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	newConf, err := decodeConfig(newNad.Spec.Config)
	if err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}
	oldConf, err := decodeConfig(oldNad.Spec.Config)
	if err != nil {
		return fmt.Errorf(updateErr, oldNad.Namespace, oldNad.Name, err)
	}
	// skip the following check if the config is not changed
	if reflect.DeepEqual(newConf, oldConf) {
		return nil
	}

	if err := v.checkNadTypes(oldConf, newConf); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	if err := v.checkNadConfigBridgeName(oldConf, newConf); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	if err := v.checkNadConfig(newConf, newNad); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	if err := v.checkVmi(newNad); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	// storagenetwork nad's params can't be changed, the only way is to clear & set storagenetwork
	// then all storagenetwork related PODs will be replaced with new nad
	if err := v.checkStorageNetwork(newNad); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	return nil
}

func (v *Validator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	nad := oldObj.(*cniv1.NetworkAttachmentDefinition)

	nadConf, err := decodeConfig(nad.Spec.Config)
	if err != nil {
		return fmt.Errorf(updateErr, nad.Namespace, nad.Name, err)
	}

	//do not delete nad when a subnet is using it
	//This will also make sure nad is not deleted when VMIs,VMs are using it
	if nadConf.Type == utils.CNITypeKubeOVN {
		if !v.subnetEnabled {
			return fmt.Errorf("operation not permitted as kubeovn is not yet enabled")
		}
		return v.checkSubnetsUsingNAD(nad)
	}

	if err := v.checkVmi(nad); err != nil {
		return fmt.Errorf(deleteErr, nad.Namespace, nad.Name, err)
	}

	// when nad is deleted, all VMs should remove the related networks&interfaces to avoid dangling
	if err := v.checkVM(nad); err != nil {
		return fmt.Errorf(deleteErr, nad.Namespace, nad.Name, err)
	}

	return nil
}

func (v *Validator) checkSubnetsUsingNAD(nad *cniv1.NetworkAttachmentDefinition) error {
	subnets, err := v.subnetCache.List(k8slabels.Everything())
	if err != nil {
		return fmt.Errorf("failed to retrieve subnets err=%v", err)
	}

	provider, err := utils.GetProviderFromNad(nad.Name, nad.Namespace)
	if err != nil {
		return err
	}

	for _, subnet := range subnets {
		if subnet.Spec.Provider == provider {
			return fmt.Errorf("subnet %s is still using the nad %s %s", subnet.Name, nad.Name, nad.Namespace)
		}
	}

	return nil
}

func (v *Validator) checkNadConfig(nadConf *utils.NetConf, nad *cniv1.NetworkAttachmentDefinition) error {
	if nadConf == nil {
		return fmt.Errorf("config is empty")
	}

	clusterNetwork := getNadClusterNetworkLabel(nad)

	if nadConf.Type == utils.CNITypeKubeOVN && clusterNetwork != "" && clusterNetwork != utils.ManagementClusterNetworkName {
		return fmt.Errorf("nad with kubeovn type can only be part of mgmt cluster")
	}

	//skip bridge config validation for type kube-ovn
	if nadConf.Type == utils.CNITypeKubeOVN {
		nadName, nadNamespace, err := utils.GetNadNameFromProvider(nadConf.Provider)
		if err != nil {
			return err
		}

		if nad.Name != nadName || nad.Namespace != nadNamespace {
			return fmt.Errorf("invalid provider name %s %s for nad %s %s", nadName, nadNamespace, nad.Name, nad.Namespace)
		}

		return nil
	}

	// The VLAN value of untagged network will be empty or number 0.
	if nadConf.Vlan < 0 || nadConf.Vlan > 4094 {
		return fmt.Errorf("VLAN ID must >=0 and <=4094")
	}

	// check and get the bridge name
	cnName, err := iface.GetBridgeNamePrefix(nadConf.BrName)
	if err != nil {
		return err
	}

	// if there is clusterNetwork label, then check if it matchs the bridge name
	if clusterNetwork != "" && cnName != clusterNetwork {
		return fmt.Errorf("the nad label %s does not match bridge name %s", clusterNetwork, cnName)
	}

	// check if clusternetwork exists
	cn, err := v.cnCache.Get(cnName)
	if err != nil {
		return fmt.Errorf("nad refers to a none-existing cluster network %s or error %w", cnName, err)
	}

	// for new NAD, the mutator patchs the MTU
	// for updated NAD, the MTU should keep same with cluster network
	targetMTU := utils.DefaultMTU
	getMtu := false

	// get MTU from clusternetwork
	if mtuStr, ok := cn.Annotations[utils.KeyUplinkMTU]; ok {
		mtu, err := utils.GetMTUFromAnnotation(mtuStr)
		if err != nil {
			return fmt.Errorf("nad's host cluster network %v has invalid MTU annotation %v/%v %w", cn.Name, utils.KeyUplinkMTU, mtuStr, err)
		}
		if mtu != 0 {
			targetMTU = mtu
		}
		getMtu = true
	}

	// get MTU value from vlanconfig
	if !getMtu {
		vcs, err := v.vcCache.List(k8slabels.Set(map[string]string{
			utils.KeyClusterNetworkLabel: clusterNetwork,
		}).AsSelector())
		if err != nil {
			return err
		}

		// if there is no vlanconfig, use default; in other case, use the first one
		if len(vcs) != 0 {
			mtu := utils.GetMTUFromVlanConfig(vcs[0])
			if utils.IsValidMTU(mtu) && mtu != 0 {
				targetMTU = mtu
			}
		}
	}

	if !utils.AreEqualMTUs(targetMTU, nadConf.MTU) {
		return fmt.Errorf("nad MTU %v does not match cluster network MTU %v", nadConf.MTU, targetMTU)
	}

	return nil
}

func (v *Validator) checkNadTypes(oldNC, newNC *utils.NetConf) error {
	if oldNC == nil {
		return fmt.Errorf("old nad config is empty")
	}

	if newNC == nil {
		return fmt.Errorf("new nad config is empty")
	}

	//nad cannot be changed from bridge to ovn and vice-versa
	if oldNC.Type != newNC.Type {
		return fmt.Errorf("nad cannot be updated from network type %s to network type %s", oldNC.Type, newNC.Type)
	}

	return nil
}

// BrName can't be changed
func (v *Validator) checkNadConfigBridgeName(oldNC, newNC *utils.NetConf) error {
	if oldNC == nil {
		return fmt.Errorf("old nad config is empty")
	}

	if newNC == nil {
		return fmt.Errorf("new nad config is empty")
	}

	if oldNC.BrName != newNC.BrName {
		return fmt.Errorf("nad bridge name can't be changed from %v to %v", oldNC.BrName, newNC.BrName)
	}

	return nil
}

func (v *Validator) checkRoute(config string) error {
	_, err := utils.NewLayer3NetworkConf(config)
	return err
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

func (v *Validator) checkVM(nad *cniv1.NetworkAttachmentDefinition) error {
	vmGetter := utils.NewVMGetter(v.vmCache)

	if vmStrList, err := vmGetter.VMNamesWhoUseNad(nad); err != nil {
		return err
	} else if len(vmStrList) > 0 {
		return fmt.Errorf("it's still used by VM(s) %s which must remove the related networks and interfaces", strings.Join(vmStrList, ", "))
	}

	return nil
}

func getNadClusterNetworkLabel(nad *cniv1.NetworkAttachmentDefinition) string {
	if nad == nil || nad.Labels == nil {
		return ""
	}
	return nad.Labels[utils.KeyClusterNetworkLabel]
}

func (v *Validator) checkStorageNetwork(nad *cniv1.NetworkAttachmentDefinition) error {
	if utils.IsStorageNetworkNad(nad) {
		return fmt.Errorf(storageNetworkErr)
	}

	return nil
}

func (v *Validator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"network-attachment-definitions"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   cniv1.SchemeGroupVersion.Group,
		APIVersion: cniv1.SchemeGroupVersion.Version,
		ObjectType: &cniv1.NetworkAttachmentDefinition{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

// decode config string to a config struct
func decodeConfig(config string) (*utils.NetConf, error) {
	conf := &utils.NetConf{}
	if config == "" {
		return conf, nil
	}

	if err := json.Unmarshal([]byte(config), &conf); err != nil {
		return nil, fmt.Errorf("unmarshal config %s failed: %w", config, err)
	}

	return conf, nil
}
