package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
)

func TestStringToBondAdSelect(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected netlink.BondAdSelect
	}{
		{
			name:     "stable",
			input:    "stable",
			expected: netlink.BOND_AD_SELECT_STABLE,
		},
		{
			name:     "bandwidth",
			input:    "bandwidth",
			expected: netlink.BOND_AD_SELECT_BANDWIDTH,
		},
		{
			name:     "count",
			input:    "count",
			expected: netlink.BOND_AD_SELECT_COUNT,
		},
		{
			name:     "invalid",
			input:    "invalid-select",
			expected: -1,
		},
		{
			name:     "empty string",
			input:    "",
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringToBondAdSelect(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestStringToBondArpValidate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected netlink.BondArpValidate
	}{
		{name: "none", input: "none", expected: netlink.BOND_ARP_VALIDATE_NONE},
		{name: "active", input: "active", expected: netlink.BOND_ARP_VALIDATE_ACTIVE},
		{name: "backup", input: "backup", expected: netlink.BOND_ARP_VALIDATE_BACKUP},
		{name: "all", input: "all", expected: netlink.BOND_ARP_VALIDATE_ALL},
		{name: "invalid", input: "invalid-validate", expected: -1},
		{name: "empty string", input: "", expected: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringToBondArpValidate(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestStringToBondArpAllTargets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected netlink.BondArpAllTargets
	}{
		{
			name:     "any",
			input:    "any",
			expected: netlink.BOND_ARP_ALL_TARGETS_ANY,
		},
		{
			name:     "all",
			input:    "all",
			expected: netlink.BOND_ARP_ALL_TARGETS_ALL,
		},
		{
			name:     "invalid",
			input:    "invalid-target",
			expected: -1,
		},
		{
			name:     "empty string",
			input:    "",
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringToBondArpAllTargets(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}
