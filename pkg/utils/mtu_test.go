package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

func TestGetMTUFromString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    int
		expectError bool
	}{
		{
			name:        "zero is valid",
			input:       "0",
			expected:    0,
			expectError: false,
		},
		{
			name:        "DefaultMTU is valid",
			input:       "1500",
			expected:    1500,
			expectError: false,
		},
		{
			name:        "MinMTU is valid",
			input:       "576",
			expected:    MinMTU,
			expectError: false,
		},
		{
			name:        "MaxMTU is valid",
			input:       "9000",
			expected:    MaxMTU,
			expectError: false,
		},
		{
			name:        "below MinMTU returns error",
			input:       "575",
			expected:    0,
			expectError: true,
		},
		{
			name:        "above MaxMTU returns error",
			input:       "9001",
			expected:    0,
			expectError: true,
		},
		{
			name:        "negative value returns error",
			input:       "-1",
			expected:    0,
			expectError: true,
		},
		{
			name:        "non-integer string returns error",
			input:       "abc",
			expected:    0,
			expectError: true,
		},
		{
			name:        "empty string returns error",
			input:       "",
			expected:    0,
			expectError: true,
		},
		{
			name:        "float string returns error",
			input:       "1500.5",
			expected:    0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetMTUFromString(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestGetMTUFromVlanConfig(t *testing.T) {
	tests := []struct {
		name     string
		vc       *networkv1.VlanConfig
		expected int
	}{
		{
			name:     "nil VlanConfig returns 0",
			vc:       nil,
			expected: 0,
		},
		{
			name: "nil LinkAttrs returns 0",
			vc: &networkv1.VlanConfig{
				Spec: networkv1.VlanConfigSpec{
					Uplink: networkv1.Uplink{
						LinkAttrs: nil,
					},
				},
			},
			expected: 0,
		},
		{
			name: "zero MTU in LinkAttrs returns 0",
			vc: &networkv1.VlanConfig{
				Spec: networkv1.VlanConfigSpec{
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{MTU: 0},
					},
				},
			},
			expected: 0,
		},
		{
			name: "valid MTU is returned",
			vc: &networkv1.VlanConfig{
				Spec: networkv1.VlanConfigSpec{
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{MTU: 1500},
					},
				},
			},
			expected: 1500,
		},
		{
			name: "jumbo frame MTU is returned",
			vc: &networkv1.VlanConfig{
				Spec: networkv1.VlanConfigSpec{
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{MTU: 9000},
					},
				},
			},
			expected: 9000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetMTUFromVlanConfig(tt.vc)
			assert.Equal(t, tt.expected, got, "Should return %d", tt.expected)
		})
	}
}

func TestMTUDefaultTo(t *testing.T) {
	tests := []struct {
		name     string
		mtu      int
		expected int
	}{
		{
			name:     "zero returns DefaultMTU",
			mtu:      0,
			expected: DefaultMTU,
		},
		{
			name:     "DefaultMTU returns DefaultMTU",
			mtu:      DefaultMTU,
			expected: DefaultMTU,
		},
		{
			name:     "non-zero value is returned as-is",
			mtu:      9000,
			expected: 9000,
		},
		{
			name:     "MinMTU is returned as-is",
			mtu:      MinMTU,
			expected: MinMTU,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MTUDefaultTo(tt.mtu)
			assert.Equal(t, tt.expected, got, "Should return %d", tt.mtu, tt.expected)
		})
	}
}
