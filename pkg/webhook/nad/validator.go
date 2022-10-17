package nad

import (
	"encoding/json"
	"fmt"
	"strings"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/yaocw2020/webhook/pkg/types"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	createErr = "could not create nad %s/%s because %w"
	deleteErr = "could not delete nad %s/%s because %w"
)

type nadValidator struct {
	types.DefaultValidator
	vmCache ctlkubevirtv1.VirtualMachineCache
}

var _ types.Validator = &nadValidator{}

func NewNadValidator(vmCache ctlkubevirtv1.VirtualMachineCache) *nadValidator {
	return &nadValidator{
		vmCache: vmCache,
	}
}

func (n *nadValidator) Create(_ *types.Request, newObj runtime.Object) error {
	netAttachDef := newObj.(*cniv1.NetworkAttachmentDefinition)

	config := netAttachDef.Spec.Config
	if config == "" {
		return fmt.Errorf(createErr, netAttachDef.Namespace, netAttachDef.Name, fmt.Errorf("spec.config is empty"))
	}

	var bridgeConf = &utils.NetConf{}
	err := json.Unmarshal([]byte(config), &bridgeConf)
	if err != nil {
		return fmt.Errorf(createErr, netAttachDef.Namespace, netAttachDef.Name, err)
	}

	if bridgeConf.Vlan < 0 || bridgeConf.Vlan > 4094 {
		return fmt.Errorf(createErr, netAttachDef.Namespace, netAttachDef.Name, fmt.Errorf("VLAN ID must >=1 and <=4094"))
	}

	lenOfBrName := len(bridgeConf.BrName)
	if lenOfBrName > iface.MaxDeviceNameLen {
		return fmt.Errorf(createErr, netAttachDef.Namespace, netAttachDef.Name, fmt.Errorf("the length of the brName could not be more than 15"))
	}
	if lenOfBrName < 3 || bridgeConf.BrName[lenOfBrName-3:] != iface.BridgeSuffix {
		return fmt.Errorf(createErr, netAttachDef.Namespace, netAttachDef.Name, fmt.Errorf("the suffix of the brName should be -br"))
	}

	return nil
}

func (n *nadValidator) Delete(_ *types.Request, oldObj runtime.Object) error {
	netAttachDef := oldObj.(*cniv1.NetworkAttachmentDefinition)

	// multus network name can be <networkName> or <namespace>/<networkName>
	// ref: https://github.com/kubevirt/client-go/blob/148fa0d1c7e83b7a56606a7ca92394ba6768c9ac/api/v1/schema.go#L1436-L1439
	networkName := fmt.Sprintf("%s/%s", netAttachDef.Namespace, netAttachDef.Name)
	vms, err := n.vmCache.GetByIndex(indexeres.VMByNetworkIndex, networkName)
	if err != nil {
		return fmt.Errorf(deleteErr, netAttachDef.Namespace, networkName, err)
	}
	if vmsTmp, err := n.vmCache.GetByIndex(indexeres.VMByNetworkIndex, netAttachDef.Name); err != nil {
		return fmt.Errorf(deleteErr, netAttachDef.Namespace, networkName, err)
	} else {
		vms = append(vms, vmsTmp...)
	}

	if len(vms) > 0 {
		vmNameList := make([]string, 0, len(vms))
		for _, vm := range vms {
			vmNameList = append(vmNameList, vm.Name)
		}
		return fmt.Errorf(deleteErr, netAttachDef.Namespace, netAttachDef.Name, fmt.Errorf("it's still used by vm(s): %s", strings.Join(vmNameList, ", ")))
	}

	return nil
}

func (n *nadValidator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"network-attachment-definitions"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   cniv1.SchemeGroupVersion.Group,
		APIVersion: cniv1.SchemeGroupVersion.Version,
		ObjectType: &cniv1.NetworkAttachmentDefinition{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Delete,
		},
	}
}
