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

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	createErr = "could not create nad %s/%s because %w"
	updateErr = "could not update nad %s/%s because %w"
	deleteErr = "could not delete nad %s/%s because %w"
)

type Validator struct {
	admission.DefaultValidator
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache
	vmCache  ctlkubevirtv1.VirtualMachineCache
}

var _ admission.Validator = &Validator{}

func NewNadValidator(vmiCache ctlkubevirtv1.VirtualMachineInstanceCache,
	vmCache ctlkubevirtv1.VirtualMachineCache) *Validator {
	return &Validator{
		vmiCache: vmiCache,
		vmCache:  vmCache,
	}
}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	nad := newObj.(*cniv1.NetworkAttachmentDefinition)

	if err := v.checkRoute(nad.Annotations[utils.KeyNetworkRoute]); err != nil {
		return fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}

	conf, err := encodeConfig(nad.Spec.Config)
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

	if err := v.checkRoute(newNad.Annotations[utils.KeyNetworkRoute]); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	newConf, err := encodeConfig(newNad.Spec.Config)
	if err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}
	oldConf, err := encodeConfig(oldNad.Spec.Config)
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

	return nil
}

func (v *Validator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	nad := oldObj.(*cniv1.NetworkAttachmentDefinition)

	if err := v.checkVmi(nad); err != nil {
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
		return fmt.Errorf("the length of the brName could not be more than 15")
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
	vmiGetter := utils.VmiGetter{VmiCache: v.vmiCache, VMCache: v.vmCache}
	vmis, err := vmiGetter.WhoUseNad(nad, nil)
	if err != nil {
		return err
	}

	if len(vmis) > 0 {
		vmiNameList := make([]string, 0, len(vmis))
		for _, vmi := range vmis {
			vmiNameList = append(vmiNameList, vmi)
		}
		return fmt.Errorf("VM(s) %s still attached", strings.Join(vmiNameList, ", "))
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

func encodeConfig(config string) (*utils.NetConf, error) {
	conf := &utils.NetConf{}
	if config == "" {
		return conf, nil
	}

	if err := json.Unmarshal([]byte(config), &conf); err != nil {
		return nil, fmt.Errorf("unmarshal config %s failed: %w", config, err)
	}

	return conf, nil
}
