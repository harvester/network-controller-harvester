package iface

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"

	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	testCN = "test"
)

func Test_GetManuallyConfiguredVlans(t *testing.T) {
	tests := []struct {
		name      string
		cnName    string
		returnErr bool
		errKey    string
		linkname1 string
		linkname2 string
		vids      []uint16
	}{
		{
			name:      "get 2 configured vids on mgmt",
			cnName:    utils.ManagementClusterNetworkName,
			returnErr: false,
			errKey:    "",
			linkname1: "mgmt-br.40", // valid format
			linkname2: "mgmt-br.50", // valid format
			vids:      []uint16{40, 50},
		},
		{
			name:      "get 1 configured vids on mgmt",
			cnName:    utils.ManagementClusterNetworkName,
			returnErr: false,
			errKey:    "",
			linkname1: "mgmt-br.1",  // valid format, but we don't work on it
			linkname2: "mgmt-br.50", // valid format
			vids:      []uint16{50},
		},
		{
			name:      "get 2 configured vids on test clusternetwork",
			cnName:    testCN,
			returnErr: false,
			errKey:    "",
			linkname1: "test-br.1000", // valid format
			linkname2: "test-br.2000", // valid format
			vids:      []uint16{1000, 2000},
		},
		{
			name:      "get 0 vids on mgmt",
			cnName:    utils.ManagementClusterNetworkName,
			returnErr: false,
			errKey:    "",
			linkname1: "mgmt-br.0",    // should not occur in real environment
			linkname2: "mgmt-br.5000", // should not occur in real environment
			vids:      []uint16{},
		},
		{
			name:      "get 1 vids on mgmt",
			cnName:    utils.ManagementClusterNetworkName,
			returnErr: false,
			errKey:    "",
			linkname1: "mgmt-br.0",
			linkname2: "mgmt-br.300", // valid format
			vids:      []uint16{300},
		},
		{
			name:      "get 0 vids on mgmt, invalid format",
			cnName:    utils.ManagementClusterNetworkName,
			returnErr: false,
			errKey:    "",
			linkname1: "mgmt.0",
			linkname2: "mgmt.300", // not the expected format, should be mgm-br.300
			vids:      []uint16{},
		},
		{
			name:      "get 0 vids on mgmt, invalid format 2",
			cnName:    utils.ManagementClusterNetworkName,
			returnErr: false,
			errKey:    "",
			linkname1: "mgmt-br.a",   // not the expected format, should be mgm-br.300
			linkname2: "mgmt-br.a.b", // not the expected format, should be mgm-br.300
			vids:      []uint16{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			link1 := &netlink.Bridge{}
			link1.LinkAttrs.Name = tc.linkname1
			link2 := &netlink.Bridge{}
			link2.LinkAttrs.Name = tc.linkname2
			links := []netlink.Link{
				link1,
				link2,
			}
			vids := getManuallyConfiguredVlans(tc.cnName, links)
			assert.True(t, reflect.DeepEqual(vids, tc.vids))
		})
	}
}
