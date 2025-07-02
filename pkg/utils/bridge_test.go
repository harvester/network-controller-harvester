package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	cn2 = "cn2"
)

func TestBridgeRelatedFunctions(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
	}{
		{
			name:      "Test basic functions of bridge",
			returnErr: false,
			errKey:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, HasMgmtClusterNetworkDevicePrefix("mgmt-br.2025"))
			assert.False(t, HasMgmtClusterNetworkDevicePrefix("mgmt-b0.2025"))
			assert.False(t, HasMgmtClusterNetworkDevicePrefix("ens0.2025"))
			assert.True(t, GetClusterNetworkDevicePrefix(ManagementClusterNetworkName) == "mgmt-br.")

			prefix := GetClusterNetworkDevicePrefix(cn2)
			assert.True(t, prefix == "cn2-br.")
			assert.True(t, HasClusterNetworkDevicePrefix(prefix, cn2))
			assert.True(t, HasClusterNetworkDevicePrefix("cn2-br.2025", prefix))
			assert.False(t, HasClusterNetworkDevicePrefix("cn22-br.", prefix))
			assert.False(t, HasClusterNetworkDevicePrefix("cn22-br.2025", prefix))
			assert.False(t, HasClusterNetworkDevicePrefix("cn22.2025", prefix))
		})
	}
}
