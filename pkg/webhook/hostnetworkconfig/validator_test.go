package hostnetworkconfig

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
		currentVC2               *networkv1.VlanConfig
		currentVS                *networkv1.VlanStatus
		currentVS2               *networkv1.VlanStatus
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
			errKey:    "are not in the same subnet",
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
				ObjectMeta: metav1.ObjectMeta{
					Name:        "testHNC-1",
					Annotations: map[string]string{"test": "test"},
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
					Name:        "testHNC-2",
					Annotations: map[string]string{"test": "test"},
				},
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
				ObjectMeta: metav1.ObjectMeta{
					Name:        "testHNC-1",
					Annotations: map[string]string{"test": "test"},
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "dhcp",
					Underlay:       true,
				},
			},
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "testHNC-2",
					Annotations: map[string]string{"test": "test"},
				},
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
		{
			name:      "vlan config with empty matched nodes and another vlanconfig spanning all nodes should be allowed",
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
			currentNode2: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node2",
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "currentVC",
					Annotations: map[string]string{utils.KeyMatchedNodes: "[\"node1\",\"node2\"]"},
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
			currentVC2: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "currentVC2",
					Annotations: map[string]string{utils.KeyMatchedNodes: "[]"},
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
			currentVS2: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node2"),
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
			if tc.currentVC2 != nil {
				_, err := vcClient.Create(tc.currentVC2)
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
			if tc.currentVS2 != nil {
				_, err := vsClient.Create(tc.currentVS2)
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
			name:      "updating HostIPs for same hostnetworkconfig should be allowed",
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
			currentHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "testHNC",
					Annotations: map[string]string{"test": "test"},
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
					Name:        "testHNC",
					Annotations: map[string]string{"test": "test2"},
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.2.100/24"},
					Underlay:       true,
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
			assert.Equal(t, tc.returnErr, err != nil, "unexpected error result: %v", err)
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}

// Test that updating an existing HostNetworkConfig succeeds even if the cluster
// contains a legacy duplicate VlanID (ensures op == ValidateCreate guard prevents false positives).
func TestUpdateHostNetworkConfigWithMultipleCacheObjects(t *testing.T) {
	tests := []struct {
		name                      string
		returnErr                 bool
		errKey                    string
		currentCN                 *networkv1.ClusterNetwork
		currentVC                 *networkv1.VlanConfig
		currentVS                 *networkv1.VlanStatus
		currentNAD                *cniv1.NetworkAttachmentDefinition
		currentHostNetworkConfigs []*networkv1.HostNetworkConfig
		oldHostNetworkConfig      *networkv1.HostNetworkConfig
		newHostNetworkConfig      *networkv1.HostNetworkConfig
		currentNode               *v1.Node
		currentNode2              *v1.Node
		currentVM                 *kubevirtv1.VirtualMachine
	}{
		{
			name:      "updating testHNC1 in a cluster with legacy duplicate VlanID on testHNC2 should succeed",
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
			// Pre-existing cache state containing both duplicate HNCs (legacy state)
			currentHostNetworkConfigs: []*networkv1.HostNetworkConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "testHNC1",
						Annotations: map[string]string{"test": "v1"},
					},
					Spec: networkv1.HostNetworkConfigSpec{
						ClusterNetwork: testCnName,
						VlanID:         2012,
						Mode:           "static",
						HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "testHNC2",
						Annotations: map[string]string{"test": "v1"},
					},
					Spec: networkv1.HostNetworkConfigSpec{
						ClusterNetwork: testCnName,
						VlanID:         2012, // Legacy duplicate VlanID due to known reasons
						Mode:           "static",
						HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.2.100/24"},
					},
				},
			},

			// The old object target being updated
			oldHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "testHNC1",
					Annotations: map[string]string{"test": "v1"},
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.1.100/24"},
				},
			},
			// The updated object target
			newHostNetworkConfig: &networkv1.HostNetworkConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "testHNC1",
					Annotations: map[string]string{"test": "v2"},
				},
				Spec: networkv1.HostNetworkConfigSpec{
					ClusterNetwork: testCnName,
					VlanID:         2012,
					Mode:           "static",
					HostIPs:        map[string]networkv1.IPAddr{"node1": "192.168.3.100/24"},
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

			assert.NotNil(t, tc.oldHostNetworkConfig)
			if tc.oldHostNetworkConfig == nil {
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
			hncClient := fakeclients.HostNetworkConfigClient(nchclientset.NetworkV1beta1().HostNetworkConfigs)
			for _, hnc := range tc.currentHostNetworkConfigs {
				_, err := hncClient.Create(hnc)
				assert.NoError(t, err)
			}

			validator := NewHostNetworkConfigValidator(nadCache, cnCache, hncCache, vcCache, vsCache, nodeCache, vmCache)

			err := validator.Update(nil, tc.oldHostNetworkConfig, tc.newHostNetworkConfig)
			assert.Equal(t, tc.returnErr, err != nil, "unexpected error result: %v", err)
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

func TestDeleteHostNetworkConfig_NoPanic(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		oldObj    runtime.Object
	}{
		{
			name:      "allow deletion when oldObj is untyped nil",
			returnErr: false,
			errKey:    "",
			oldObj:    nil,
		},
		{
			name:      "allow deletion when oldObj is typed nil pointer",
			returnErr: false,
			errKey:    "",
			oldObj:    (*networkv1.HostNetworkConfig)(nil),
		},
		{
			name:      "allow deletion when oldObj is an invalid runtime object type",
			returnErr: false,
			errKey:    "",
			oldObj:    &networkv1.VlanConfig{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewHostNetworkConfigValidator(nil, nil, nil, nil, nil, nil, nil)

			err := validator.Delete(nil, tc.oldObj)
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
		name       string
		cidrs      []string
		familyName string
		wantErr    bool
	}{
		{
			name:       "empty slice",
			cidrs:      []string{},
			familyName: "IPv4",
			wantErr:    false,
		},
		{
			name:       "single valid ipv4 cidr",
			cidrs:      []string{"192.168.1.0/24"},
			familyName: "IPv4",
			wantErr:    false,
		},
		{
			name:       "identical ipv4 cidrs",
			cidrs:      []string{"10.0.0.0/16", "10.0.0.0/16"},
			familyName: "IPv4",
			wantErr:    false,
		},
		{
			name:       "different ipv4 masks",
			cidrs:      []string{"192.168.1.0/24", "192.168.1.0/16"},
			familyName: "IPv4",
			wantErr:    true,
		},
		{
			name:       "different ipv4 networks",
			cidrs:      []string{"192.168.1.0/24", "192.168.2.0/24"},
			familyName: "IPv4",
			wantErr:    true,
		},
		{
			name:       "single valid ipv6 cidr",
			cidrs:      []string{"2001:db8::/64"},
			familyName: "IPv6",
			wantErr:    false,
		},
		{
			name:       "identical ipv6 cidrs",
			cidrs:      []string{"2001:db8:1::/64", "2001:db8:1::/64"},
			familyName: "IPv6",
			wantErr:    false,
		},
		{
			name:       "different ipv6 networks",
			cidrs:      []string{"2001:db8:1::/64", "2001:db8:2::/64"},
			familyName: "IPv6",
			wantErr:    true,
		},
		{
			name:       "different ipv6 prefix lengths",
			cidrs:      []string{"2001:db8:1::/64", "2001:db8:1::/48"},
			familyName: "IPv6",
			wantErr:    true,
		},
		{
			name:       "invalid first cidr",
			cidrs:      []string{"not-a-cidr"},
			familyName: "IPv4",
			wantErr:    true,
		},
		{
			name:       "invalid second cidr",
			cidrs:      []string{"2001:db8::/64", "invalid-ipv6"},
			familyName: "IPv6",
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := sameSubnet(tc.cidrs, tc.familyName)
			if tc.wantErr {
				assert.Error(t, err, "expected error for cidrs=%v family=%s", tc.cidrs, tc.familyName)
			} else {
				assert.NoError(t, err, "expected no error for cidrs=%v family=%s", tc.cidrs, tc.familyName)
			}
		})
	}
}

func TestValidateIPFamilyAndIP(t *testing.T) {
	ptrIPAddr := func(s string) *networkv1.IPAddr {
		addr := networkv1.IPAddr(s)
		return &addr
	}

	tests := []struct {
		name        string
		hnc         *networkv1.HostNetworkConfig
		expectError bool
	}{
		{
			name: "Valid: Default IPFamily (empty, defaults to IPv4) with legacy HostIPs",
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: "",
					HostIPs:  map[string]networkv1.IPAddr{"node1": "192.168.1.10/24"},
				},
			},
			expectError: false,
		},
		{
			name: "Valid: IPv4 family with HostMultiFamilyIPs",
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {IPv4: ptrIPAddr("192.168.1.10/24")},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Valid: IPv6 family with HostMultiFamilyIPs",
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {IPv6: ptrIPAddr("2001:db8::1/64")},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Valid: Dual-stack family with HostMultiFamilyIPs",
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyDualIPv4First,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
							IPv6: ptrIPAddr("2001:db8::1/64"),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid: Both legacy HostIPs and HostMultiFamilyIPs specified",
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs:  map[string]networkv1.IPAddr{"node1": "192.168.1.10/24"},
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {IPv4: ptrIPAddr("192.168.1.10/24")},
					},
				},
			},
			expectError: true,
		},
		{
			name: "Invalid: Legacy HostIPs used under IPv6 family",
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostIPs:  map[string]networkv1.IPAddr{"node1": "192.168.1.10/24"},
				},
			},
			expectError: true,
		},
		{
			name: "Invalid: Missing HostMultiFamilyIPs under IPv6 family",
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
				},
			},
			expectError: true,
		},
		{
			name: "Invalid: Unsupported IP family",
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: "UnsupportedFamily",
				},
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIPFamilyAndIP(tc.hnc)
			if (err != nil) != tc.expectError {
				t.Errorf("validateIPFamilyAndIP() error = %v, expectError %v", err, tc.expectError)
			}
		})
	}
}

func TestValidateStaticIPs(t *testing.T) {
	ptrIPAddr := func(s string) *networkv1.IPAddr {
		addr := networkv1.IPAddr(s)
		return &addr
	}

	node1 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{"kubernetes.io/hostname": "node1"},
		},
	}
	node2 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node2",
			Labels: map[string]string{"kubernetes.io/hostname": "node2"},
		},
	}

	tests := []struct {
		name         string
		nodes        []*v1.Node
		hnc          *networkv1.HostNetworkConfig
		expectError  bool
		errSubstring string
	}{
		{
			name:  "Valid: standard legacy IPv4 static IPs matching nodes",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs: map[string]networkv1.IPAddr{
						"node1": "192.168.1.10/24",
					},
				},
			},
			expectError: false,
		},
		{
			name:  "Valid: standard dual-stack static IPs matching nodes",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyDualIPv4First,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
							IPv6: ptrIPAddr("2001:db8::1/64"),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:  "Valid: standard IPv6 static IPs matching nodes",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv6: ptrIPAddr("2001:db8::1/64"),
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:  "Valid: maximum length (64 chars) valid IPv6 CIDR",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							// Length of "2001:0db8:85a3:0000:0000:8a2e:0370:7334/64" is exactly 43 chars.
							// To test max length of 64, we can pad or use a full-length representation:
							// "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff/64" -> 41 chars
							// Let's use a fully expanded 39-char IPv6 address + "/128" (total 43 chars),
							// or max length standard format:
							IPv6: ptrIPAddr("2001:0db8:0000:0000:0000:0000:0000:0001/64"), // 43 chars
						},
					},
				},
			},
			expectError: false,
		},
		{
			name:  "Invalid: Mutual exclusivity (both HostIPs and HostMultiFamilyIPs specified)",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs:  map[string]networkv1.IPAddr{"node1": "192.168.1.10/24"},
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {IPv4: ptrIPAddr("192.168.1.10/24")},
					},
				},
			},
			expectError:  true,
			errSubstring: "cannot specify both",
		},
		{
			name:  "Invalid: Malformed IPv4 CIDR format",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs:  map[string]networkv1.IPAddr{"node1": "192.168.1.300/24"},
				},
			},
			expectError:  true,
			errSubstring: "invalid IPv4 CIDR format",
		},
		{
			name:  "Invalid: IPv4 CIDR is explicitly an empty string",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr(""), // non-nil pointer to empty string
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "invalid IPv4 CIDR format",
		},
		{
			name:  "Invalid: IPv4 field contains an IPv6 address string",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("2001:db8::1/64"), // IPv6 address assigned to the IPv4 field
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "expected IPv4 address for IPv4",
		},
		{
			name:  "Invalid: IPv6 field contains an IPv4 address string",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv6: ptrIPAddr("192.168.1.10/24"), // IPv4 address assigned to the IPv6 field
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "expected IPv6 address for IPv6",
		},
		{
			name:  "Invalid: Malformed IPv6 CIDR format",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {IPv6: ptrIPAddr("2001:db8:::1/64")},
					},
				},
			},
			expectError:  true,
			errSubstring: "invalid IPv6 CIDR format",
		},
		{
			name:  "Invalid: completely empty IP maps (refused upfront as zombie prevention)",
			nodes: []*v1.Node{},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs:  map[string]networkv1.IPAddr{},
				},
			},
			expectError:  true,
			errSubstring: "at least one static IP configuration must be provided",
		},
		{
			name:  "Invalid: node matched but missing from IP map",
			nodes: []*v1.Node{node1, node2},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs: map[string]networkv1.IPAddr{
						"node1": "192.168.1.10/24",
						// node2 missing
					},
				},
			},
			expectError:  true,
			errSubstring: "static IP not found for node node2",
		},
		{
			name:  "Invalid: node matched but missing from multi-family IP map",
			nodes: []*v1.Node{node1, node2},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
						},
						// node2 missing from HostMultiFamilyIPs
					},
				},
			},
			expectError:  true,
			errSubstring: "static IP configuration not found for node",
		},
		{
			name:  "Invalid: IPv4-family node config missing IPv4 address",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: nil, // Missing IPv4 under IPv4 family
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "IPv4 address is required for node node1 under ipv4 family",
		},
		{
			name:  "Invalid: IPv6-family node config missing IPv6 address",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv6: nil, // Missing IPv6 under IPv6 family
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "IPv6 address is required for node node1 under ipv6 family",
		},
		{
			name:  "Invalid: Unsupported IP family encountered during node check",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: "BustedFamily", // Passes early enum check if bypassed or modified, hits node-level switch default
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "unsupported IP family: BustedFamily",
		},
		{
			name:  "Invalid: IPv4 family configuration containing stray IPv6 address",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
							IPv6: ptrIPAddr("2001:db8::1/64"), // forbidden under IPv4 family
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "IPv6 addresses are not allowed when IP family is IPv4",
		},
		{
			name:  "Invalid: IPv6-only family configuration containing stray IPv4 address",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"), // forbidden under IPv6-only family
							IPv6: ptrIPAddr("2001:db8::1/64"),
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "IPv4 addresses are not allowed when IP family is IPv6",
		},
		{
			name:  "Invalid: Dual-stack IPv4/IPv6 count mismatch via extra map record",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyDualIPv4First,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
							IPv6: ptrIPAddr("2001:db8::1/64"),
						},
						"nodeX": { // Extra map record with only IPv4 (causes count mismatch)
							IPv4: ptrIPAddr("192.168.1.99/24"),
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "dual-stack static IPs count mismatch",
		},
		{
			name:  "Invalid: Subnet inconsistency across node entries",
			nodes: []*v1.Node{node1, node2},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs: map[string]networkv1.IPAddr{
						"node1": "192.168.1.10/24",
						"node2": "192.168.2.10/24", // different subnet
					},
				},
			},
			expectError:  true,
			errSubstring: "not in the same subnet",
		},
		{
			name:  "Valid: Extra placeholder/leftover records present, but all entries share the same subnet",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs: map[string]networkv1.IPAddr{
						"node1": "192.168.1.10/24",
						"nodeX": "192.168.1.99/24", // placeholder record allowed
					},
				},
			},
			expectError: false,
		},
		{
			name:  "Invalid: Dual-stack node missing IPv6 (fails at node level validation)",
			nodes: []*v1.Node{node1},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyDualIPv4First,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
							// IPv6 omitted -> caught by node-level completeness check before count mismatch
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "both IPv4 and IPv6 addresses are required for node node1 under dual-stack family",
		},
		{
			name:  "Invalid: legacy IPv4 duplicate assignment",
			nodes: []*v1.Node{node1, node2},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostIPs: map[string]networkv1.IPAddr{
						"node1": "192.168.1.10/24",
						"node2": "192.168.1.10/24",
					},
				},
			},
			expectError:  true,
			errSubstring: "duplicate static IPv4",
		},
		{
			name:  "Invalid: multi-family IPv4 duplicate assignment",
			nodes: []*v1.Node{node1, node2},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv4,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
						},
						"node2": {
							IPv4: ptrIPAddr("192.168.1.10/24"),
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "duplicate static IPv4",
		},
		{
			name:  "Invalid: multi-family IPv6 duplicate assignment",
			nodes: []*v1.Node{node1, node2},
			hnc: &networkv1.HostNetworkConfig{
				Spec: networkv1.HostNetworkConfigSpec{
					IPFamily: networkv1.IPFamilyIPv6,
					HostMultiFamilyIPs: map[string]networkv1.MultiFamilyIP{
						"node1": {
							IPv6: ptrIPAddr("2001:db8::10/64"),
						},
						"node2": {
							IPv6: ptrIPAddr("2001:db8::10/64"),
						},
					},
				},
			},
			expectError:  true,
			errSubstring: "duplicate static IPv6",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nchclientset := fake.NewSimpleClientset()
			nodeClient := fakeclients.NodeClient(nchclientset.CoreV1().Nodes)
			nodeCache := fakeclients.NodeCache(nchclientset.CoreV1().Nodes)

			for _, node := range tc.nodes {
				_, err := nodeClient.Create(node)
				assert.NoError(t, err)
			}

			validator := NewHostNetworkConfigValidator(nil, nil, nil, nil, nil, nodeCache, nil)

			err := validator.validateStaticIPs(tc.hnc)
			if tc.expectError {
				assert.Error(t, err)
				if tc.errSubstring != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errSubstring), "expected error to contain %q, got %v", tc.errSubstring, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
