package nad

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/harvester/webhook/pkg/types"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type networkType string

const (
	l2VlanNetwork   networkType = "L2VlanNetwork"
	untaggedNetwork networkType = "UntaggedNetwork"
)

type nadMutator struct {
	types.DefaultMutator
}

var _ types.Mutator = &nadMutator{}

func NewNadMutator() *nadMutator {
	return &nadMutator{}
}

func (n *nadMutator) Create(_ *types.Request, newObj runtime.Object) (types.Patch, error) {
	netAttachDef := newObj.(*cniv1.NetworkAttachmentDefinition)

	labels := netAttachDef.Labels
	if labels == nil {
		labels = make(map[string]string)
	}

	netconf := &utils.NetConf{}
	if err := json.Unmarshal([]byte(netAttachDef.Spec.Config), netconf); err != nil {
		return nil, fmt.Errorf(createErr, netAttachDef.Namespace, netAttachDef.Name, err)
	}

	klog.V(5).Infof("netconf: %+v", netconf)

	if netconf.Vlan != 0 {
		labels[utils.KeyNetworkType] = string(l2VlanNetwork)
		labels[utils.KeyVlanLabel] = strconv.Itoa(netconf.Vlan)
	} else {
		labels[utils.KeyNetworkType] = string(untaggedNetwork)
	}

	lenOfBrName := len(netconf.BrName)
	if lenOfBrName < len(iface.BridgeSuffix) {
		return nil, fmt.Errorf(createErr, netAttachDef.Namespace, netAttachDef.Name,
			fmt.Errorf("the length of brName must be more than %d", len(iface.BridgeSuffix)))
	}
	labels[utils.KeyClusterNetworkLabel] = netconf.BrName[:lenOfBrName-len(iface.BridgeSuffix)]

	return types.Patch{
		types.PatchOp{
			Op:    types.PatchOpReplace,
			Path:  "/metadata/labels",
			Value: labels,
		}}, nil
}

func (n *nadMutator) Resource() types.Resource {
	return types.Resource{
		Names:      []string{"network-attachment-definitions"},
		Scope:      admissionregv1.NamespacedScope,
		APIGroup:   cniv1.SchemeGroupVersion.Group,
		APIVersion: cniv1.SchemeGroupVersion.Version,
		ObjectType: &cniv1.NetworkAttachmentDefinition{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
		},
	}
}
