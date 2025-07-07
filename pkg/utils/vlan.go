package utils

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	MaxVlanID      = 4094
	MinVlanID      = 0
	MinTrunkVlanID = 1

	VlanIDCount = 4096
)

type VlanIDSet struct {
	ClusterNetwork string `json:"clusterNetwork"`
	VlanCount      uint32 `json:"vlanCount"`
	VIDSetHash     string `json:"vidSetHash,omitempty"`
	VIDSet         []bool `json:"vidSet,omitempty"` // store vlan list
	Vlan           int    `json:"vlan"`
}

func (vis *VlanIDSet) SetVID(vid int) error {
	if vid < MinVlanID || vid > MaxVlanID {
		return fmt.Errorf("vlan %v is out of range [%v .. %v]", vid, MinVlanID, MaxVlanID)
	}
	if vid == 0 || vid == 1 {
		return nil
	}

	if len(vis.VIDSet) < (vid + 1) {
		fmt.Errorf("vlan set length is %v, not enough space to store vid %v", len(vis.VIDSet), vid)
	}
	if vis.VIDSet[vid] == false {
		vis.VIDSet[vid] = true
		vis.VlanCount++
	}
	return nil
}

func (vis *VlanIDSet) Append(other *VlanIDSet) *VlanIDSet {
	if other == nil {
		return vis
	}
	if len(vis.VIDSet) == 0 {
		vis.VIDSet = make([]bool, VlanIDCount)
	}
	for i := range other.VIDSet {
		if other.VIDSet[i] {
			vis.VIDSet[i] = true
			vis.VlanCount++
		}
	}
	return vis
}

func (vis *VlanIDSet) VidSetToStr() string {
	if vis == nil || len(vis.VIDSet) == 0 || vis.VlanCount == 0 {
		return ""
	}
	tgt := make([]string, vis.VlanCount)
	k := 0

	for i := range vis.VIDSet {
		if vis.VIDSet[i] {
			tgt[k] = strconv.Itoa(i)
			k++
		}
	}
	return strings.Join(tgt, " ")
}

func newVlanIDSet() *VlanIDSet {
	vis := &VlanIDSet{}
	vis.VIDSet = make([]bool, VlanIDCount)
	return vis
}
