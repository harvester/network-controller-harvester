package utils

import (
	"crypto/sha256"
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
	clusterNetwork string   `json:"clusterNetwork"`
	vlanCount      uint32   `json:"vlanCount"`
	vidSetHash     string   `json:"vidSetHash,omitempty"`
	vidSet         []bool   `json:"vidSet,omitempty"`  // store vlan list
	cidrSet        []string `json:"cidrSet,omitempty"` // store vlan cidr
	vlan           int      `json:"vlan"`
}

// moved from pkg/network/vlan to here
type LocalArea struct {
	Vid  uint16
	Cidr string
}

func (vis *VlanIDSet) SetVID(vid int) error {
	if vid < MinVlanID || vid > MaxVlanID {
		return fmt.Errorf("vlan %v is out of range [%v .. %v]", vid, MinVlanID, MaxVlanID)
	}
	// 0, 1 are always skipped
	if vid == 0 || vid == 1 {
		return nil
	}

	if len(vis.vidSet) < (vid + 1) {
		fmt.Errorf("vlan set length is %v, not enough space to store vid %v", len(vis.vidSet), vid)
	}
	if vis.vidSet[vid] == false {
		vis.vidSet[vid] = true
		vis.vlanCount++
	}
	return nil
}

// merge another VlandIDSet to current
func (vis *VlanIDSet) Append(other *VlanIDSet) *VlanIDSet {
	if other == nil {
		return vis
	}

	upper := len(other.vidSet)
	if upper > VlanIDCount {
		upper = VlanIDCount
	}
	for i := range upper {
		if other.vidSet[i] {
			if !vis.vidSet[i] {
				vis.vidSet[i] = true
				vis.cidrSet[i] = other.cidrSet[i]
				vis.vlanCount++
			} else if vis.cidrSet[i] == "" {
				// only copy cidr from another, e.g. vid 100 is first added as a trunk vid, then as an access vid with cidr
				vis.cidrSet[i] = other.cidrSet[i]
			}
		}
	}
	return vis
}

func (vis *VlanIDSet) VidSetToString() string {
	if vis == nil || len(vis.vidSet) == 0 || vis.vlanCount == 0 {
		return ""
	}
	tgt := make([]string, vis.vlanCount)
	k := 0

	for i := range vis.vidSet {
		if vis.vidSet[i] {
			tgt[k] = strconv.Itoa(i)
			k++
		}
	}
	return strings.Join(tgt, ",")
}

func (vis *VlanIDSet) ToLocalAreas() []LocalArea {
	if vis == nil || len(vis.vidSet) == 0 || vis.vlanCount == 0 {
		return nil
	}
	tgt := make([]LocalArea, vis.vlanCount)
	k := 0

	for i := range vis.vidSet {
		if vis.vidSet[i] {
			tgt[k].Vid = uint16(i) // nolint: gosec
			tgt[k].Cidr = vis.cidrSet[i]
			k++
		}
	}
	return tgt
}

func (vis *VlanIDSet) VidSetToStringHash() (str, hash string) {
	str = vis.VidSetToString()
	bs := sha256.Sum256([]byte(str))
	hash = fmt.Sprintf("%x", bs)
	return
}

func NewVlanIDSet() *VlanIDSet {
	vis := &VlanIDSet{
		vidSet:  make([]bool, VlanIDCount),
		cidrSet: make([]string, VlanIDCount),
	}
	// 1 are always set
	vis.vidSet[1] = true
	vis.vlanCount = 1
	return vis
}

func GetLocalArea(vlanIDStr, routeConf string) (*LocalArea, error) {
	vlanID, err := strconv.Atoi(vlanIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid vlan id %s", vlanIDStr)
	}

	layer3NetworkConf := &Layer3NetworkConf{}
	if routeConf != "" {
		if layer3NetworkConf, err = NewLayer3NetworkConf(routeConf); err != nil {
			return nil, err
		}
	}

	return &LocalArea{
		Vid:  uint16(vlanID), //nolint:gosec
		Cidr: layer3NetworkConf.CIDR,
	}, nil
}
