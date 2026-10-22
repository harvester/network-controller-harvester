package utils

import (
	"k8s.io/apimachinery/pkg/labels"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
)

// IsIsolatedClusterNetwork reports whether any live VlanConfig of the cluster network uses the isolated uplink mode.
// The VlanConfigs are the source of truth; the KeyUplinkModeLabel on the cluster network only mirrors them.
func IsIsolatedClusterNetwork(cnName string, vcCache ctlnetworkv1.VlanConfigCache) (bool, error) {
	if cnName == "" || cnName == ManagementClusterNetworkName {
		return false, nil
	}
	vcs, err := vcCache.List(labels.Set{KeyClusterNetworkLabel: cnName}.AsSelector())
	if err != nil {
		return false, err
	}
	for _, vc := range vcs {
		if vc.DeletionTimestamp == nil && vc.Spec.Uplink.IsIsolated() {
			return true, nil
		}
	}
	return false, nil
}

// UplinkModeOf returns the effective uplink mode of a VlanConfig ("" is reported as bond)
func UplinkModeOf(vc *networkv1.VlanConfig) networkv1.UplinkMode {
	if vc.Spec.Uplink.IsIsolated() {
		return networkv1.UplinkModeIsolated
	}
	return networkv1.UplinkModeBond
}
