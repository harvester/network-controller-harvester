package vlanconfig

import (
	"context"
	"net"
	"testing"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

func TestValidateUplinkMode(t *testing.T) {
	tests := []struct {
		name    string
		cn      string
		mode    networkv1.UplinkMode
		nics    []string
		mac     string
		wantErr bool
	}{
		{name: "default mode", cn: "storage", nics: []string{"eth1"}},
		{name: "bond mode", cn: "storage", mode: networkv1.UplinkModeBond, nics: []string{"eth1", "eth2"}},
		{name: "isolated mode", cn: "storage", mode: networkv1.UplinkModeIsolated, nics: []string{"eth1", "eth2"}},
		{name: "isolated mode without NICs", cn: "storage", mode: networkv1.UplinkModeIsolated, wantErr: true},
		{name: "isolated mode on mgmt", cn: utils.ManagementClusterNetworkName, mode: networkv1.UplinkModeIsolated, nics: []string{"eth1"}, wantErr: true},
		{name: "unknown mode", cn: "storage", mode: "mesh", nics: []string{"eth1"}, wantErr: true},
		{name: "isolated mode with a hardware address", cn: "storage", mode: networkv1.UplinkModeIsolated, nics: []string{"eth1"}, mac: "02:00:00:00:00:01", wantErr: true},
		{name: "bond mode with a hardware address", cn: "storage", nics: []string{"eth1"}, mac: "02:00:00:00:00:01"},
	}

	for _, tc := range tests {
		vc := &networkv1.VlanConfig{Spec: networkv1.VlanConfigSpec{
			ClusterNetwork: tc.cn,
			Uplink:         networkv1.Uplink{NICs: tc.nics, Mode: tc.mode},
		}}
		if tc.mac != "" {
			mac, err := net.ParseMAC(tc.mac)
			if err != nil {
				t.Fatal(err)
			}
			vc.Spec.Uplink.LinkAttrs = &networkv1.LinkAttrs{HardwareAddr: mac}
		}
		if err := validateUplinkMode(vc); (err != nil) != tc.wantErr {
			t.Errorf("%s: validateUplinkMode() error = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestCheckUplinkModeOnClusterNetwork(t *testing.T) {
	vcOn := func(name, cn string, mode networkv1.UplinkMode) *networkv1.VlanConfig {
		return &networkv1.VlanConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{utils.KeyClusterNetworkLabel: cn}},
			Spec: networkv1.VlanConfigSpec{ClusterNetwork: cn,
				Uplink: networkv1.Uplink{NICs: []string{"eth1", "eth2"}, Mode: mode}},
		}
	}
	nadOn := func(ns, name, cn string) *cniv1.NetworkAttachmentDefinition {
		return &cniv1.NetworkAttachmentDefinition{ObjectMeta: metav1.ObjectMeta{
			Namespace: ns, Name: name, Labels: map[string]string{utils.KeyClusterNetworkLabel: cn}}}
	}

	tests := []struct {
		name     string
		existVCs []*networkv1.VlanConfig
		nads     []*cniv1.NetworkAttachmentDefinition
		hncs     []*networkv1.HostNetworkConfig
		newVC    *networkv1.VlanConfig
		wantErr  bool
	}{
		{name: "first isolated vlanconfig on an empty cluster network", newVC: vcOn("vc1", "storage", networkv1.UplinkModeIsolated)},
		{name: "isolated next to isolated", existVCs: []*networkv1.VlanConfig{vcOn("vc0", "storage", networkv1.UplinkModeIsolated)},
			newVC: vcOn("vc1", "storage", networkv1.UplinkModeIsolated)},
		{name: "bond next to isolated", existVCs: []*networkv1.VlanConfig{vcOn("vc0", "storage", networkv1.UplinkModeIsolated)},
			newVC: vcOn("vc1", "storage", ""), wantErr: true},
		{name: "isolated next to bond", existVCs: []*networkv1.VlanConfig{vcOn("vc0", "storage", networkv1.UplinkModeBond)},
			newVC: vcOn("vc1", "storage", networkv1.UplinkModeIsolated), wantErr: true},
		{name: "isolated with only the storage network on it",
			nads:  []*cniv1.NetworkAttachmentDefinition{nadOn(utils.HarvesterSystemNamespaceName, "storagenetwork-abcde", "storage")},
			newVC: vcOn("vc1", "storage", networkv1.UplinkModeIsolated)},
		{name: "isolated while a VM network uses the cluster network",
			nads:  []*cniv1.NetworkAttachmentDefinition{nadOn("default", "vm-net", "storage")},
			newVC: vcOn("vc1", "storage", networkv1.UplinkModeIsolated), wantErr: true},
		{name: "bond while a VM network uses the cluster network",
			nads:  []*cniv1.NetworkAttachmentDefinition{nadOn("default", "vm-net", "data")},
			newVC: vcOn("vc1", "data", "")},
		{name: "isolated while a host network config uses the cluster network",
			hncs:  []*networkv1.HostNetworkConfig{{ObjectMeta: metav1.ObjectMeta{Name: "hnc"}, Spec: networkv1.HostNetworkConfigSpec{ClusterNetwork: "storage", VlanID: 10, Mode: "dhcp"}}},
			newVC: vcOn("vc1", "storage", networkv1.UplinkModeIsolated), wantErr: true},
		{name: "isolated while a host network config uses another cluster network",
			hncs:  []*networkv1.HostNetworkConfig{{ObjectMeta: metav1.ObjectMeta{Name: "hnc"}, Spec: networkv1.HostNetworkConfigSpec{ClusterNetwork: "data", VlanID: 10, Mode: "dhcp"}}},
			newVC: vcOn("vc1", "storage", networkv1.UplinkModeIsolated)},
		{name: "the vlanconfig itself is not compared with itself", existVCs: []*networkv1.VlanConfig{vcOn("vc1", "storage", networkv1.UplinkModeBond)},
			newVC: vcOn("vc1", "storage", networkv1.UplinkModeIsolated)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			for _, vc := range tc.existVCs {
				if _, err := cs.NetworkV1beta1().VlanConfigs().Create(context.TODO(), vc, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			for _, nad := range tc.nads {
				if _, err := cs.K8sCniCncfIoV1().NetworkAttachmentDefinitions(nad.Namespace).Create(context.TODO(), nad, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			for _, hnc := range tc.hncs {
				if _, err := cs.NetworkV1beta1().HostNetworkConfigs().Create(context.TODO(), hnc, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			v := &Validator{
				nadCache: fakeclients.NetworkAttachmentDefinitionCache(cs.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
				vcCache:  fakeclients.VlanConfigCache(cs.NetworkV1beta1().VlanConfigs),
				hncCache: fakeclients.HostNetworkConfigCache(cs.NetworkV1beta1().HostNetworkConfigs),
			}
			if err := v.checkUplinkModeOnClusterNetwork(tc.newVC); (err != nil) != tc.wantErr {
				t.Errorf("checkUplinkModeOnClusterNetwork() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
