package nad

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/webhook/pkg/server/admission"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubevirtv1 "kubevirt.io/api/core/v1"

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
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache
	cnCache  ctlnetworkv1.ClusterNetworkCache
}

var _ admission.Validator = &Validator{}

func NewNadValidator(vmiCache ctlkubevirtv1.VirtualMachineInstanceCache, cnCache ctlnetworkv1.ClusterNetworkCache) *Validator {
	return &Validator{
		vmiCache: vmiCache,
		cnCache:  cnCache,
	}
}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	nad := newObj.(*cniv1.NetworkAttachmentDefinition)

	// check clusternetwrork
	if err := v.checkClusterNetwork(nad); err != nil {
		return fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}

	if err := v.checkRoute(nad.Annotations[utils.KeyNetworkRoute]); err != nil {
		return fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}

	conf, err := decodeConfig(nad.Spec.Config)
	if err != nil {
		return fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}
	if err := v.checkNadConfig(conf); err != nil {
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

	// check clusternetwrork
	if err := v.checkClusterNetworkUnchanged(oldNad, newNad); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	// check clusternetwrork
	if err := v.checkClusterNetwork(newNad); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
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
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}
	// skip the update if the config is not changed
	if reflect.DeepEqual(newConf, oldConf) {
		return nil
	}
	if err := v.checkNadConfig(newConf); err != nil {
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

	if err := v.checkVmi(nad); err != nil {
		return fmt.Errorf(deleteErr, nad.Namespace, nad.Name, err)
	}

	// storagenetwork can't be deleted by user, only the harvester storagenetwork controller can
	if err := v.checkStorageNetwork(nad); err != nil {
		return fmt.Errorf(deleteErr, nad.Namespace, nad.Name, err)
	}

	return nil
}

func (v *Validator) checkNadConfig(bridgeConf *utils.NetConf) error {
	if bridgeConf == nil {
		return fmt.Errorf("config is empty")
	}

	// The VLAN value of untagged network will be empty or number 0.
	if bridgeConf.Vlan < 0 || bridgeConf.Vlan > 4094 {
		return fmt.Errorf("VLAN ID must >=0 and <=4094")
	}

	lenOfBrName, lenOfBridgeSuffix := len(bridgeConf.BrName), len(iface.BridgeSuffix)
	if lenOfBrName > iface.MaxDeviceNameLen {
		return fmt.Errorf("the length of the brName can't be more than %v", iface.MaxDeviceNameLen)
	}
	if lenOfBrName <= lenOfBridgeSuffix || bridgeConf.BrName[lenOfBrName-lenOfBridgeSuffix:] != iface.BridgeSuffix {
		return fmt.Errorf("the suffix of the brName should be -br")
	}

	return nil
}

func (v *Validator) checkRoute(config string) error {
	_, err := utils.NewLayer3NetworkConf(config)
	return err
}

func (v *Validator) checkVmi(nad *cniv1.NetworkAttachmentDefinition) error {
	vmiGetter := utils.VmiGetter{VmiCache: v.vmiCache}
	vmis, err := vmiGetter.WhoUseNad(nad, nil)
	if err != nil {
		return err
	}

	return v.generateVmiNoneStopError(nad, vmis)
}

func (v *Validator) checkClusterNetwork(nad *cniv1.NetworkAttachmentDefinition) error {
	if nad.Labels == nil {
		return fmt.Errorf("nad does not have label")
	}

	cnName := nad.Labels[utils.KeyClusterNetworkLabel]
	if cnName == "" {
		return fmt.Errorf("nad has empty cluster network name label")
	}

	if _, err := v.cnCache.Get(cnName); err != nil {
		return fmt.Errorf("nad refers to none-existing cluster network %s", cnName)
	}

	return nil
}

func (v *Validator) checkClusterNetworkUnchanged(oldNad, newNad *cniv1.NetworkAttachmentDefinition) error {
	if oldNad.Labels == nil {
		return fmt.Errorf("old nad does not have label")
	}

	if newNad.Labels == nil {
		return fmt.Errorf("new nad does not have label")
	}

	if oldNad.Labels[utils.KeyClusterNetworkLabel] != newNad.Labels[utils.KeyClusterNetworkLabel] {
		return fmt.Errorf("nad network can't be changed from %s to %s", oldNad.Labels[utils.KeyClusterNetworkLabel], newNad.Labels[utils.KeyClusterNetworkLabel])
	}

	if oldNad.Labels[utils.KeyNetworkType] != newNad.Labels[utils.KeyNetworkType] {
		return fmt.Errorf("nad network type can't be changed from %s to %s", oldNad.Labels[utils.KeyNetworkType], newNad.Labels[utils.KeyNetworkType])
	}

	return nil
}

func (v *Validator) checkStorageNetwork(nad *cniv1.NetworkAttachmentDefinition) error {
	if utils.IsStorageNetworkNad(nad) {
		fmt.Errorf(storageNetworkErr)
	}

	return nil
}

// for convenicen of test code
func (v *Validator) generateVmiNoneStopError(_ *cniv1.NetworkAttachmentDefinition, vmis []*kubevirtv1.VirtualMachineInstance) error {
	if len(vmis) > 0 {
		vmiNameList := make([]string, 0, len(vmis))
		for _, vmi := range vmis {
			vmiNameList = append(vmiNameList, vmi.Namespace+"/"+vmi.Name)
		}
		return fmt.Errorf("it's still used by VM(s) %s which must be stopped at first", strings.Join(vmiNameList, ", "))
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
