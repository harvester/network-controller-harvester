package vlanidset

import (
	"fmt"

	"github.com/vishvananda/netlink"
	"k8s.io/klog"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
)

const VlanIDSetCount = 4096

type VlanIDSet struct {
	ClusterNetwork string
	VlanCount      uint32
	VIDSetHash     string
	VIDSet         *[]bool
}


func GetNadVid(nads *v1.NetworkAttachmentDefinitionList) (uint16, error) {
	vlanIDStr = nad.Labels[utils.KeyVlanLabel]
	if vlanIDStr == "" {
		return 0, nil
	}

	vlanID, err := strconv.Atoi(vlanIDStr)
	if err != nil {
		return 0, fmt.Errorf("invalid vlan id string %s", vlanIDStr)
	}

	if vlanID > 4094 {
		return 0, fmt.Errorf("invalid vlan id value %s", vlanIDStr)
	}

	return vlanID, nil
}

func NewVlanIDSet(cn string, nads *v1.NetworkAttachmentDefinitionList) (*VlanIDSet, error) {
	if len(nads) == 0 {
		return &VlanIDSet{
			ClusterNetwork: cn,
			VlanCount: 0,
			VIDSetHash: "",
		}
	}

	vids := &{
		ClusterNetwork: cn,
		VIDSet: make([]bool, VlanIDSetCount)
	}

	vids[0] = true
	vids[1] = true
	vids.VlanCount = 2
	
	cnt := 0
	for _, nad := range nads {
		vid, _ := GetNadVid(nads)
		if vids.VIDSet[vid] == false {
			vids.VIDSet[vid] = true
			cnt += 1
		}
	}
	
	return vids, nil
}



