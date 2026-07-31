package iface

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
	// "github.com/harvester/harvester-network-controller/pkg/utils"
)

func TestCompareBond(t *testing.T) {
	mac1, _ := net.ParseMAC("00:00:5e:00:53:01")
	// mac2, _ := net.ParseMAC("00:00:5e:00:53:02")

	baseBond := func() *netlink.Bond {
		return &netlink.Bond{
			LinkAttrs: netlink.LinkAttrs{
				Name:         "bond0",
				MTU:          1500,
				TxQLen:       1000,
				HardwareAddr: mac1,
			},
			Mode:           netlink.BOND_MODE_ACTIVE_BACKUP,
			Miimon:         100,
			XmitHashPolicy: netlink.BOND_XMIT_HASH_POLICY_LAYER2,
			LacpRate:       netlink.BOND_LACP_RATE_SLOW,
			AdSelect:       netlink.BOND_AD_SELECT_STABLE,
			ArpInterval:    0,
			ArpIpTargets:   nil,
			ArpValidate:    netlink.BOND_ARP_VALIDATE_NONE,
			ArpAllTargets:  netlink.BOND_ARP_ALL_TARGETS_ANY,
		}
	}

	testCases := []struct {
		name     string
		oldBond  *netlink.Bond
		newBond  *netlink.Bond
		expected bool
	}{
		{
			name:     "get 2 configured vids on mgmt",
			oldBond:  baseBond(),
			newBond:  baseBond(),
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compareBond(tc.oldBond, tc.newBond)
			assert.Equal(t, tc.expected, result)
		})
	}
}
