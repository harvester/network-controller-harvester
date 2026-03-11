package hostnetworkconfig

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevirtv1 "kubevirt.io/api/core/v1"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"

	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const (
	testCnName             = "test-cn"
	testCnName11           = "cn-11"
	testCNNewName          = "cn-new"
	currentHostNetworkName = "curr-host-network-config-test"
	invalidCName           = "this-clusternetwork-name-is-way-too-long-to-be-valid"
	testNadConfig          = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":2012,\"ipam\":{}}"
	testNadConfigNew       = "{\"cniVersion\":\"0.3.1\",\"name\":\"net2-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":2013,\"ipam\":{}}"
	testNadName            = "net1-vlan"
	testNadNameNew         = "net2-vlan"
	testNamespace          = "test"
	testVMName             = "vm1"
	invalidHostNetworkName = "cluster-1"
)

func TestCreateHostNetworkConfig(t *testing.T) {
	tests := []struct {
		name                     string
		returnErr                bool
		errKey                   string
		currentCN                *networkv1.ClusterNetwork
		currentCN11              *networkv1.ClusterNetwork
		currentVC                *networkv1.VlanConfig
		currentVS                *networkv1.VlanStatus
		currentNAD               *cniv1.NetworkAttachmentDefinition
		newNAD                   *cniv1.NetworkAttachmentDefinition
		newHostNetworkConfig     *networkv1.HostNetworkConfig
		currentNode              *v1.Node
		currentNode2             *v1.Node
		currentHostNetworkConfig *networkv1.HostNetworkConfig
	}{
		{
			name:      "valid hostnetworkconfig can be created",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "currentVC",
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.DefaultMTU,
						},
					},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "currentVC",
					Conditions: []networkv1.Condition{
						{
							Type:   networkv1.Ready,
							Status: "True",
						},
					},
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "invalid cluster network name",
			returnErr: true,
			errKey:    "is more than",
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: invalidCName,
					VlanID:         2012,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "invalid host network interface name due to cluster network name and vlanid",
			returnErr: true,
			errKey:    "is more than",
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: invalidHostNetworkName,
					VlanID:         2012,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "cluster network does not exist",
			returnErr: true,
			errKey:    "none-existing cluster network",
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "ips do not exist for mode static",
			returnErr: true,
			errKey:    "static IP not found for node",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node2": "192.168.1.100/24"},
				},
			},
		},
		{
			name:      "static ips not in same subnet",
			returnErr: true,
			errKey:    "static IPs are not in the same subnet",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			currentNode2: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node2",
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.0.100/24", "node2": "192.168.1.200/24"},
				},
			},
		},
		{
			name:      "same host network config exists already",
			returnErr: true,
			errKey:    "already exists",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "only one underlay should exist",
			returnErr: true,
			errKey:    "with underlay enabled",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentCN11: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName11,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadNameNew,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName11},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigNew,
				},
			},
			currentNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
					Underlay:       true,
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName11,
					VlanID:         2013,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
					Underlay:       true,
				},
			},
		},
		{
			name:      "vlan status not ready",
			returnErr: true,
			errKey:    "status is not Ready",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "currentVC",
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.DefaultMTU,
						},
					},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "currentVC",
					Conditions: []networkv1.Condition{
						{
							Type:   networkv1.Ready,
							Status: "False",
						},
					},
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "vlan config does not span all nodes when using underlay",
			returnErr: true,
			errKey:    "vlanconfig does not span",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			currentNode2: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node2",
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "currentVC",
					Annotations: map[string]string{utils.KeyMatchedNodes: "[\"node1\"]"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.DefaultMTU,
						},
					},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "currentVC",
					Conditions: []networkv1.Condition{
						{
							Type:   networkv1.Ready,
							Status: "True",
						},
					},
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
					Underlay:       true,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			assert.NotNil(t, tc.newHostNetworkConfig)
			if tc.newHostNetworkConfig == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			vmCache := fakeclients.VirtualMachineCache(nchclientset.KubevirtV1().VirtualMachines)
			nadCache := fakeclients.NetworkAttachmentDefinitionCache(nchclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			hncCache := fakeclients.HostNetworkConfigCache(nchclientset.NetworkV1beta1().HostNetworkConfigs)

			// client to inject test data
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)
			vsClient := fakeclients.VlanStatusClient(nchclientset.NetworkV1beta1().VlanStatuses)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			nodeClient := fakeclients.NodeClient(nchclientset.CoreV1().Nodes)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			vsCache := fakeclients.VlanStatusCache(nchclientset.NetworkV1beta1().VlanStatuses)
			nodeCache := fakeclients.NodeCache(nchclientset.CoreV1().Nodes)

			if tc.currentVC != nil {
				_, err := vcClient.Create(tc.currentVC)
				assert.NoError(t, err)
			}
			if tc.currentCN != nil {
				_, err := cnClient.Create(tc.currentCN)
				assert.NoError(t, err)
			}
			if tc.currentCN11 != nil {
				_, err := cnClient.Create(tc.currentCN11)
				assert.NoError(t, err)
			}
			if tc.currentVS != nil {
				_, err := vsClient.Create(tc.currentVS)
				assert.NoError(t, err)
			}
			if tc.currentNode != nil {
				_, err := nodeClient.Create(tc.currentNode)
				assert.NoError(t, err)
			}
			if tc.currentNode2 != nil {
				_, err := nodeClient.Create(tc.currentNode2)
				assert.NoError(t, err)
			}

			if tc.currentNAD != nil {
				nadGvr := schema.GroupVersionResource{
					Group:    "k8s.cni.cncf.io",
					Version:  "v1",
					Resource: "network-attachment-definitions",
				}

				if err := nchclientset.Tracker().Create(nadGvr, tc.currentNAD.DeepCopy(), tc.currentNAD.Namespace); err != nil {
					t.Fatalf("failed to add nad %+v", tc.currentNAD)
				}
			}

			if tc.newNAD != nil {
				nadGvr := schema.GroupVersionResource{
					Group:    "k8s.cni.cncf.io",
					Version:  "v1",
					Resource: "network-attachment-definitions",
				}

				if err := nchclientset.Tracker().Create(nadGvr, tc.newNAD.DeepCopy(), tc.newNAD.Namespace); err != nil {
					t.Fatalf("failed to add nad %+v", tc.newNAD)
				}
			}

			if tc.currentHostNetworkConfig != nil {
				hncClient := fakeclients.HostNetworkConfigClient(nchclientset.NetworkV1beta1().HostNetworkConfigs)
				_, err := hncClient.Create(tc.currentHostNetworkConfig)
				assert.NoError(t, err)
			}

			validator := NewHostNetworkConfigValidator(nadCache, cnCache, hncCache, vcCache, vsCache, nodeCache, vmCache)

			err := validator.Create(nil, tc.newHostNetworkConfig)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}

func TestUpdateHostNetworkConfig(t *testing.T) {
	tests := []struct {
		name                     string
		returnErr                bool
		errKey                   string
		currentCN                *networkv1.ClusterNetwork
		currentVC                *networkv1.VlanConfig
		currentVS                *networkv1.VlanStatus
		currentNAD               *cniv1.NetworkAttachmentDefinition
		newHostNetworkConfig     *networkv1.HostNetworkConfig
		currentNode              *v1.Node
		currentNode2             *v1.Node
		currentHostNetworkConfig *networkv1.HostNetworkConfig
		currentVM                *kubevirtv1.VirtualMachine
	}{
		{
			name:      "updating mode for same vlan-id and cluster network should be allowed",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "currentVC",
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.DefaultMTU,
						},
					},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "currentVC",
					Conditions: []networkv1.Condition{
						{
							Type:   networkv1.Ready,
							Status: "True",
						},
					},
				},
			},
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "updating cluster network is not allowed",
			returnErr: true,
			errKey:    "cannot update",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentHostNetworkName,
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentHostNetworkName,
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCNNewName,
					VlanID:         2012,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "updating vlan-id is not allowed",
			returnErr: true,
			errKey:    "cannot update",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentHostNetworkName,
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentHostNetworkName,
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2014,
					Mode:           "dhcp",
				},
			},
		},
		{
			name:      "cannot disable underlay when overlay VMs are using the network",
			returnErr: true,
			errKey:    "it's still used by VM(s)",
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName, utils.KeyNetworkType: string(utils.OverlayNetwork)},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentVM: &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testVMName,
					Namespace: testNamespace,
				},
				Spec: kubevirtv1.VirtualMachineSpec{
					Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
						Spec: kubevirtv1.VirtualMachineInstanceSpec{
							Networks: []kubevirtv1.Network{
								{
									Name: "nic-1",
									NetworkSource: kubevirtv1.NetworkSource{
										Multus: &kubevirtv1.MultusNetwork{
											NetworkName: testNamespace + "/" + testNadName, // same with nad namesapce
										},
									},
								},
							},
							Domain: kubevirtv1.DomainSpec{
								Devices: kubevirtv1.Devices{
									Interfaces: []kubevirtv1.Interface{
										{
											Name: "nic-1",
										},
									},
								},
							},
						}, // vmi.spec
					},
				},
			},
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentHostNetworkName,
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
					Underlay:       true,
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentHostNetworkName,
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
					Underlay:       false,
				},
			},
		},
		{
			name:      "updating ip address for same vlan-id static mode and cluster network should be allowed",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
			currentNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "currentVC",
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.DefaultMTU,
						},
					},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "currentVC",
					Conditions: []networkv1.Condition{
						{
							Type:   networkv1.Ready,
							Status: "True",
						},
					},
				},
			},
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.101/24"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			assert.NotNil(t, tc.newHostNetworkConfig)
			if tc.newHostNetworkConfig == nil {
				return
			}

			assert.NotNil(t, tc.currentHostNetworkConfig)
			if tc.currentHostNetworkConfig == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			vmCache := fakeclients.VirtualMachineCache(nchclientset.KubevirtV1().VirtualMachines)
			nadCache := fakeclients.NetworkAttachmentDefinitionCache(nchclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			hncCache := fakeclients.HostNetworkConfigCache(nchclientset.NetworkV1beta1().HostNetworkConfigs)

			// client to inject test data
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)
			vsClient := fakeclients.VlanStatusClient(nchclientset.NetworkV1beta1().VlanStatuses)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			nodeClient := fakeclients.NodeClient(nchclientset.CoreV1().Nodes)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			vsCache := fakeclients.VlanStatusCache(nchclientset.NetworkV1beta1().VlanStatuses)
			nodeCache := fakeclients.NodeCache(nchclientset.CoreV1().Nodes)

			if tc.currentVC != nil {
				_, err := vcClient.Create(tc.currentVC)
				assert.NoError(t, err)
			}
			if tc.currentCN != nil {
				_, err := cnClient.Create(tc.currentCN)
				assert.NoError(t, err)
			}
			if tc.currentVS != nil {
				_, err := vsClient.Create(tc.currentVS)
				assert.NoError(t, err)
			}
			if tc.currentNode != nil {
				_, err := nodeClient.Create(tc.currentNode)
				assert.NoError(t, err)
			}
			if tc.currentNode2 != nil {
				_, err := nodeClient.Create(tc.currentNode2)
				assert.NoError(t, err)
			}

			if tc.currentVM != nil {
				err := nchclientset.Tracker().Add(tc.currentVM)
				assert.Nil(t, err, "mock resource vm should add into fake controller tracker")
			}

			if tc.currentNAD != nil {
				nadGvr := schema.GroupVersionResource{
					Group:    "k8s.cni.cncf.io",
					Version:  "v1",
					Resource: "network-attachment-definitions",
				}

				if err := nchclientset.Tracker().Create(nadGvr, tc.currentNAD.DeepCopy(), tc.currentNAD.Namespace); err != nil {
					t.Fatalf("failed to add nad %+v", tc.currentNAD)
				}
			}

			if tc.currentHostNetworkConfig != nil {
				hncClient := fakeclients.HostNetworkConfigClient(nchclientset.NetworkV1beta1().HostNetworkConfigs)
				_, err := hncClient.Create(tc.currentHostNetworkConfig)
				assert.NoError(t, err)
			}

			validator := NewHostNetworkConfigValidator(nadCache, cnCache, hncCache, vcCache, vsCache, nodeCache, vmCache)

			err := validator.Update(nil, tc.currentHostNetworkConfig, tc.newHostNetworkConfig)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}

func TestDeleteHostNetworkConfig(t *testing.T) {
	tests := []struct {
		name                     string
		returnErr                bool
		errKey                   string
		currentHostNetworkConfig *networkv1.HostNetworkConfig
	}{
		{
			name:      "delete hostnetworkconfig successfully",
			returnErr: false,
			errKey:    "",
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
					Underlay:       false,
				},
			},
		},
		{
			name:      "cannot delete hostnetworkconfig when underlay is enabled",
			returnErr: true,
			errKey:    "disable underlay first",

			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
					Underlay:       true,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.currentHostNetworkConfig)
			if tc.currentHostNetworkConfig == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			vmCache := fakeclients.VirtualMachineCache(nchclientset.KubevirtV1().VirtualMachines)
			nadCache := fakeclients.NetworkAttachmentDefinitionCache(nchclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			hncCache := fakeclients.HostNetworkConfigCache(nchclientset.NetworkV1beta1().HostNetworkConfigs)

			// client to inject test data
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			vsCache := fakeclients.VlanStatusCache(nchclientset.NetworkV1beta1().VlanStatuses)
			nodeCache := fakeclients.NodeCache(nchclientset.CoreV1().Nodes)

			if tc.currentHostNetworkConfig != nil {
				hncClient := fakeclients.HostNetworkConfigClient(nchclientset.NetworkV1beta1().HostNetworkConfigs)
				_, err := hncClient.Create(tc.currentHostNetworkConfig)
				assert.NoError(t, err)
			}

			validator := NewHostNetworkConfigValidator(nadCache, cnCache, hncCache, vcCache, vsCache, nodeCache, vmCache)

			err := validator.Delete(nil, tc.currentHostNetworkConfig)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}

func TestSameSubnet(t *testing.T) {
	tests := []struct {
		name    string
		cidrs   []string
		wantOK  bool
		wantErr bool
	}{
		{
			name:    "empty slice",
			cidrs:   []string{},
			wantOK:  true,
			wantErr: false,
		},
		{
			name:    "single valid cidr",
			cidrs:   []string{"192.168.1.0/24"},
			wantOK:  true,
			wantErr: false,
		},
		{
			name:    "identical cidrs",
			cidrs:   []string{"10.0.0.0/16", "10.0.0.0/16"},
			wantOK:  true,
			wantErr: false,
		},
		{
			name:    "different masks",
			cidrs:   []string{"192.168.1.0/24", "192.168.1.0/16"},
			wantOK:  false,
			wantErr: false,
		},
		{
			name:    "different networks",
			cidrs:   []string{"192.168.1.0/24", "192.168.2.0/24"},
			wantOK:  false,
			wantErr: false,
		},
		{
			name:    "invalid first cidr",
			cidrs:   []string{"not-a-cidr"},
			wantOK:  false,
			wantErr: true,
		},
		{
			name:    "invalid second cidr",
			cidrs:   []string{"10.0.0.0/8", "invalid"},
			wantOK:  false,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := sameSubnet(tc.cidrs)
			if tc.wantErr {
				assert.Error(t, err, "expected error for cidrs=%v", tc.cidrs)
			} else {
				assert.NoError(t, err, "expected no error for cidrs=%v", tc.cidrs)
			}
			assert.Equal(t, tc.wantOK, ok, "expected sameSubnet to return %v for cidrs=%v, got %v", tc.wantOK, tc.cidrs, ok)
		})
	}
}
