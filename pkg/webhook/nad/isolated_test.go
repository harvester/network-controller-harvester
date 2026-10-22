package nad

import (
	"context"
	"testing"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

func TestCreateNadOnIsolatedClusterNetwork(t *testing.T) {
	const bridgeNadConfig = `{"cniVersion":"0.3.1","type":"bridge","bridge":"storage-br","promiscMode":true,"vlan":1,"mtu":9000,"ipam":{"type":"whereabouts","range":"10.26.0.0/24"}}`

	tests := []struct {
		name      string
		namespace string
		nadName   string
		isolated  bool
		wantErr   bool
	}{
		{name: "storage network nad on isolated cluster network", namespace: utils.HarvesterSystemNamespaceName, nadName: "storagenetwork-abcde", isolated: true},
		{name: "rwx network nad on isolated cluster network", namespace: utils.HarvesterSystemNamespaceName, nadName: "rwx-network-abcde", isolated: true},
		{name: "vm network nad on isolated cluster network", namespace: "default", nadName: "vm-net", isolated: true, wantErr: true},
		{name: "vm network nad on bond cluster network", namespace: "default", nadName: "vm-net"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			cn := &networkv1.ClusterNetwork{ObjectMeta: metav1.ObjectMeta{Name: "storage",
				Annotations: map[string]string{utils.KeyUplinkMTU: "9000"}}}
			if _, err := cs.NetworkV1beta1().ClusterNetworks().Create(context.TODO(), cn, metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}
			mode := networkv1.UplinkModeBond
			if tc.isolated {
				mode = networkv1.UplinkModeIsolated
			}
			vc := &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "vc", Labels: map[string]string{utils.KeyClusterNetworkLabel: "storage"}},
				Spec: networkv1.VlanConfigSpec{ClusterNetwork: "storage",
					Uplink: networkv1.Uplink{NICs: []string{"eth1", "eth2"}, Mode: mode, LinkAttrs: &networkv1.LinkAttrs{MTU: 9000}}},
			}
			if _, err := cs.NetworkV1beta1().VlanConfigs().Create(context.TODO(), vc, metav1.CreateOptions{}); err != nil {
				t.Fatal(err)
			}

			validator := NewNadValidator(
				fakeclients.VirtualMachineCache(cs.KubevirtV1().VirtualMachines),
				fakeclients.VirtualMachineInstanceCache(cs.KubevirtV1().VirtualMachineInstances),
				fakeclients.ClusterNetworkCache(cs.NetworkV1beta1().ClusterNetworks),
				fakeclients.VlanConfigCache(cs.NetworkV1beta1().VlanConfigs),
				fakeclients.SubnetCache(cs.KubeovnV1().Subnets),
				false,
				fakeclients.HostNetworkConfigCache(cs.NetworkV1beta1().HostNetworkConfigs),
				fakeclients.NetworkAttachmentDefinitionCache(cs.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
			)
			nad := &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{Namespace: tc.namespace, Name: tc.nadName,
					Labels: map[string]string{utils.KeyClusterNetworkLabel: "storage"}},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{Config: bridgeNadConfig},
			}
			if err := validator.Create(nil, nad); (err != nil) != tc.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
