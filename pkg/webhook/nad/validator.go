package nad

import (
	"encoding/json"
	"fmt"
	"strings"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/webhook/pkg/types"
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
	types.DefaultValidator
	vmiCache ctlkubevirtv1.VirtualMachineInstanceCache
}

var _ types.Validator = &Validator{}

func NewNadValidator(vmiCache ctlkubevirtv1.VirtualMachineInstanceCache) *Validator {
	return &Validator{
		vmiCache: vmiCache,
	}
}

func (v *Validator) Create(_ *types.Request, newObj runtime.Object) error {
	nad := newObj.(*cniv1.NetworkAttachmentDefinition)

	if err := v.checkNadConfig(nad.Spec.Config); err != nil {
		return fmt.Errorf(createErr, nad.Namespace, nad.Name, err)
	}

	return nil
}

func (v *Validator) Update(_ *types.Request, oldObj, newObj runtime.Object) error {
	newNad := newObj.(*cniv1.NetworkAttachmentDefinition)

	if newNad.DeletionTimestamp != nil {
		return nil
	}

	if err := v.checkNadConfig(newNad.Spec.Config); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	if err := v.checkVmi(newNad); err != nil {
		return fmt.Errorf(updateErr, newNad.Namespace, newNad.Name, err)
	}

	return nil
}

func (v *Validator) Delete(_ *types.Request, oldObj runtime.Object) error {
	nad := oldObj.(*cniv1.NetworkAttachmentDefinition)

	if err := v.checkVmi(nad); err != nil {
		return fmt.Errorf(deleteErr, nad.Namespace, nad.Name, err)
	}

	return nil
}

func (v *Validator) checkNadConfig(config string) error {
	if config == "" {
		return fmt.Errorf("config is empty")
	}

	var bridgeConf = &utils.NetConf{}
	err := json.Unmarshal([]byte(config), &bridgeConf)
	if err != nil {
		return err
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

func (v *Validator) checkVmi(nad *cniv1.NetworkAttachmentDefinition) error {
	vmiGetter := utils.VmiGetter{VmiCache: v.vmiCache}
	vmis, err := vmiGetter.WhoUseNad(nad, nil)
	if err != nil {
		return err
	}

	if len(vmis) > 0 {
		vmiNameList := make([]string, 0, len(vmis))
		for _, vmi := range vmis {
			vmiNameList = append(vmiNameList, vmi.Namespace+"/"+vmi.Name)
		}
		return fmt.Errorf("it's still used by VM(s) %s which must be stopped at first", strings.Join(vmiNameList, ", "))
	}

	return nil
}

func (v *Validator) Resource() types.Resource {
	return types.Resource{
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
