package utils

import (
	"fmt"

	mapset "github.com/deckarep/golang-set/v2"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

const (
	VMIByNetworkIndex = "vm.harvesterhci.io/vmi-by-network"
)

type VmiGetter struct {
	VmiCache ctlkubevirtv1.VirtualMachineInstanceCache
}

// WhoUseNad requires adding network indexer to the vmi cache before invoking it
func (v *VmiGetter) WhoUseNad(nad *nadv1.NetworkAttachmentDefinition, nodesFilter mapset.Set[string]) ([]*kubevirtv1.VirtualMachineInstance, error) {
	v.VmiCache.AddIndexer(VMIByNetworkIndex, vmiByNetwork)
	// multus network name can be <networkName> or <namespace>/<networkName>
	// ref: https://github.com/kubevirt/client-go/blob/148fa0d1c7e83b7a56606a7ca92394ba6768c9ac/api/v1/schema.go#L1436-L1439
	networkName := fmt.Sprintf("%s/%s", nad.Namespace, nad.Name)
	vmis, err := v.VmiCache.GetByIndex(VMIByNetworkIndex, networkName)
	if err != nil {
		return nil, err
	}

	vmisTmp, err := v.VmiCache.GetByIndex(VMIByNetworkIndex, nad.Name)
	if err != nil {
		return nil, err
	}
	for _, vmi := range vmisTmp {
		if vmi.Namespace != nad.Namespace {
			continue
		}
		vmis = append(vmis, vmi)
	}

	if nodesFilter == nil || nodesFilter.Cardinality() == 0 {
		return vmis, nil
	}

	afterFilter := make([]*kubevirtv1.VirtualMachineInstance, 0, len(vmis))
	// filter vmis whose status.nodeName is not in the nodes map
	for _, vmi := range vmis {
		if nodesFilter.Contains(vmi.Status.NodeName) {
			afterFilter = append(afterFilter, vmi)
		}
	}

	return afterFilter, nil
}

func vmiByNetwork(obj *kubevirtv1.VirtualMachineInstance) ([]string, error) {
	networks := obj.Spec.Networks
	networkNameList := make([]string, 0, len(networks))
	for _, network := range networks {
		if network.NetworkSource.Multus == nil {
			continue
		}
		networkNameList = append(networkNameList, network.NetworkSource.Multus.NetworkName)
	}
	return networkNameList, nil
}
