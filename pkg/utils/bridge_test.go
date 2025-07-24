package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBridgeRelatedFunctions(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
	}{
		{
			name:      "Test basic functions of bridge related definitions",
			returnErr: false,
			errKey:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, HasMgmtClusterNetworkDevicePrefix("mgmt-br.2025"))
			assert.False(t, HasMgmtClusterNetworkDevicePrefix("mgmt-b0.2025"))
			assert.False(t, HasMgmtClusterNetworkDevicePrefix("ens0.2025"))
		})
	}
}
