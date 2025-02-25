package utils

import (
	"fmt"

	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	VMByNetworkIndex = "vm.harvesterhci.io/vm-by-network"
)

type VMGetter struct {
	VMCache ctlkubevirtv1.VirtualMachineCache
}

// WhoUseNad requires adding network indexer to the vm cache before invoking it
func (v *VMGetter) WhoUseNad(nad *nadv1.NetworkAttachmentDefinition) ([]*kubevirtv1.VirtualMachine, error) {
	v.VMCache.AddIndexer(VMByNetworkIndex, vmByNetwork)
	// multus network name can be <networkName> or <namespace>/<networkName>
	// ref: https://github.com/kubevirt/client-go/blob/148fa0d1c7e83b7a56606a7ca92394ba6768c9ac/api/v1/schema.go#L1436-L1439
	networkName := fmt.Sprintf("%s/%s", nad.Namespace, nad.Name)
	vms, err := v.VMCache.GetByIndex(VMByNetworkIndex, networkName)
	if err != nil {
		return nil, err
	}

	vmsTmp, err := v.VMCache.GetByIndex(VMByNetworkIndex, nad.Name)
	if err != nil {
		return nil, err
	}
	for _, vm := range vmsTmp {
		if vm.Namespace != nad.Namespace {
			continue
		}
		vms = append(vms, vm)
	}

	return vms, nil
}

func vmByNetwork(obj *kubevirtv1.VirtualMachine) ([]string, error) {
	networks := obj.Spec.Template.Spec.Networks
	networkNameList := make([]string, 0, len(networks))
	for _, network := range networks {
		if network.NetworkSource.Multus == nil {
			continue
		}
		networkNameList = append(networkNameList, network.NetworkSource.Multus.NetworkName)
	}
	return networkNameList, nil
}
