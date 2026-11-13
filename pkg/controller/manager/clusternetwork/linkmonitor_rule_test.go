package clusternetwork

import (
	"context"
	"regexp"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

func TestLinkMonitorNameRule(t *testing.T) {
	vc := func(name string, mode networkv1.UplinkMode, nics ...string) *networkv1.VlanConfig {
		return &networkv1.VlanConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{utils.KeyClusterNetworkLabel: "storage"}},
			Spec:       networkv1.VlanConfigSpec{ClusterNetwork: "storage", Uplink: networkv1.Uplink{NICs: nics, Mode: mode}},
		}
	}

	tests := []struct {
		name     string
		vcs      []*networkv1.VlanConfig
		want     string
		match    []string
		notMatch []string
	}{
		{name: "bond mode keeps the existing rule", vcs: []*networkv1.VlanConfig{vc("vc", "", "eth1", "eth2")},
			want: "storage(-br|-bo)", match: []string{"storage-br", "storage-bo"}},
		{name: "isolated mode adds the NICs of all vlanconfigs",
			vcs:      []*networkv1.VlanConfig{vc("vc1", networkv1.UplinkModeIsolated, "eth2", "eth1"), vc("vc2", networkv1.UplinkModeIsolated, "eth1", "enp1s0f1")},
			want:     "^storage(-br|-bo)$|^(enp1s0f1|eth1|eth2)$",
			match:    []string{"storage-br", "eth1", "eth2", "enp1s0f1"},
			notMatch: []string{"eth10", "veth1234", "mgmt-br", "storage-br.100"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			for _, v := range tc.vcs {
				if _, err := cs.NetworkV1beta1().VlanConfigs().Create(context.TODO(), v, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			h := Handler{vcCache: fakeclients.VlanConfigCache(cs.NetworkV1beta1().VlanConfigs)}
			got, err := h.linkMonitorNameRule("storage")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("linkMonitorNameRule() = %q, want %q", got, tc.want)
			}
			for _, n := range tc.match {
				if ok, _ := regexp.MatchString(got, n); !ok {
					t.Errorf("rule %q should match %q", got, n)
				}
			}
			for _, n := range tc.notMatch {
				if ok, _ := regexp.MatchString(got, n); ok {
					t.Errorf("rule %q should not match %q", got, n)
				}
			}
		})
	}
}
