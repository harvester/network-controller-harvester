package nad

import (
	"strings"
	"testing"

	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevirtv1 "kubevirt.io/api/core/v1"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"

	harvesterfake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"

	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const (
	testCnName            = "test-cn"
	testNadConfig         = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}"
	testNadConfig1        = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn1-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}"
	testNadName           = "net1-vlan"
	testVMName            = "vm1"
	testNamespace         = "test"
	testKubeOVNNadName    = "vswitch1"
	testKubeOVNNamespace  = "default"
	testKubeOVNNadConfig  = "{\"cniVersion\":\"0.3.1\",\"name\":\"vswitch1\",\"type\":\"kube-ovn\",\"server_socket\":\"/run/openvswitch/kube-ovn-daemon.sock\", \"provider\": \"vswitch1.default.ovn\"}"
	testKubeOVNNadConfig1 = "{\"cniVersion\":\"0.3.1\",\"name\":\"vswitch1\",\"type\":\"kube-ovn\",\"server_socket\":\"/run/openvswitch/kube-ovn-daemon.sock\"}"
	testKubeOVNNadConfig2 = "{\"cniVersion\":\"0.3.1\",\"name\":\"vswitch1\",\"type\":\"kube-ovn\",\"server_socket\":\"/run/openvswitch/kube-ovn-daemon.sock\", \"provider\": \"vswitch7.default.ovn\"}"
	testSubnetName        = "vswitch1"
	testSubnetNamespace   = "default"
)

func TestCreateNAD(t *testing.T) {
	tests := []struct {
		name       string
		returnErr  bool
		errKey     string
		currentCN  *networkv1.ClusterNetwork
		currentVC  *networkv1.VlanConfig
		currentNAD *cniv1.NetworkAttachmentDefinition
		newNAD     *cniv1.NetworkAttachmentDefinition
	}{
		{
			name:      "valid NAD can be created",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
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
		},
		{
			name:      "valid NAD can be created when it does not have label",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfig,
				},
			},
		},
		{
			name:      "NAD can't be created as it's config is an invalid JSON string",
			returnErr: true,
			errKey:    "unmarshal config",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: "test-cn"},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					// config is invalid
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridgebridge\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it's label does not match bridge name",
			returnErr: true,
			errKey:    "does not match",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: "test-cn-mismatch"}, // does not match bridge name
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it's label and config refer to a none-existing cluster network",
			returnErr: true,
			errKey:    "none-existing",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: "none"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"none-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it's config refers to a none-existing cluster network",
			returnErr: true,
			errKey:    "none-existing",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"none-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it has invalid config string",
			returnErr: true,
			errKey:    "unmarshal",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it has invalid VLAN id which is -1",
			returnErr: true,
			errKey:    "VLAN ID must",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":-1,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it has invalid VLAN id which is 4095",
			returnErr: true,
			errKey:    "VLAN ID must",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":4095,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it has too long bridge name",
			returnErr: true,
			errKey:    "the length of the brName",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"harvester-br-too-long\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it has too short bridge name",
			returnErr: true,
			errKey:    "the suffix of the brName",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"a\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it has empty bridge name",
			returnErr: true,
			errKey:    "the suffix of the brName",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as has invalid bridge suffix",
			returnErr: true,
			errKey:    "the suffix of the brName",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"suffix-br-\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as the host cluster network has invalid mtu annotation",
			returnErr: true,
			errKey:    "has invalid MTU annotation",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "15000"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "valid NAD of type kube-ovn can be created",
			returnErr: false,
			errKey:    "",
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testKubeOVNNadName,
					Namespace: testKubeOVNNamespace,
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testKubeOVNNadConfig,
				},
			},
		},
		{
			name:      "empty provider name type kube-ovn returns error",
			returnErr: true,
			errKey:    "provider is empty",
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testKubeOVNNadName,
					Namespace: testKubeOVNNamespace,
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testKubeOVNNadConfig1,
				},
			},
		},
		{
			name:      "invalid provider name type kube-ovn returns error",
			returnErr: true,
			errKey:    "invalid provider name",
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testKubeOVNNadName,
					Namespace: testKubeOVNNamespace,
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testKubeOVNNadConfig2,
				},
			},
		},
		{
			name:      "invalid provider namespace type kube-ovn returns error",
			returnErr: true,
			errKey:    "invalid provider name",
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testKubeOVNNadName,
					Namespace: "testdefault",
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testKubeOVNNadConfig,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			assert.NotNil(t, tc.newNAD)
			if tc.newNAD == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()

			vmCache := fakeclients.VirtualMachineCache(harvesterclientset.KubevirtV1().VirtualMachines)
			vmiCache := fakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)

			// client to inject test data
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			subnetCache := fakeclients.SubnetCache(nchclientset.KubeovnV1().Subnets)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}

			validator := NewNadValidator(vmCache, vmiCache, cnCache, vcCache, subnetCache, true)

			err := validator.Create(nil, tc.newNAD)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}

func TestUpdateNAD(t *testing.T) {
	tests := []struct {
		name       string
		returnErr  bool
		errKey     string
		currentNAD *cniv1.NetworkAttachmentDefinition
	}{
		{
			name:      "change the cluster network on nad throws error",
			returnErr: true,
			errKey:    "nad bridge name can't be changed",
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
		},
		{
			name:      "change the nad type (bridge/kube-ovn) throws error",
			returnErr: true,
			errKey:    "nad cannot be updated",
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testKubeOVNNadName,
					Namespace: testKubeOVNNamespace,
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testKubeOVNNadConfig,
				},
			},
		},
	}

	oldNad := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testNadName,
			Namespace:   testNamespace,
			Annotations: map[string]string{"test": "test"},
			Labels:      map[string]string{utils.KeyClusterNetworkLabel: "test-cn1"},
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			Config: testNadConfig1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.currentNAD)
			if tc.currentNAD == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			vmCache := fakeclients.VirtualMachineCache(harvesterclientset.KubevirtV1().VirtualMachines)
			vmiCache := fakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			subnetCache := fakeclients.SubnetCache(nchclientset.KubeovnV1().Subnets)
			validator := NewNadValidator(vmCache, vmiCache, cnCache, vcCache, subnetCache, true)

			nadGvr := schema.GroupVersionResource{
				Group:    "k8s.cni.cncf.io",
				Version:  "v1",
				Resource: "network-attachment-definitions",
			}

			if err := harvesterclientset.Tracker().Create(nadGvr, oldNad.DeepCopy(), oldNad.Namespace); err != nil {
				t.Fatalf("failed to add nad %+v", oldNad)
			}

			err := validator.Update(nil, oldNad, tc.currentNAD)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				if err != nil {
					assert.True(t, strings.Contains(err.Error(), tc.errKey))
				}
			}
		})
	}
}

func TestDeleteNAD(t *testing.T) {
	tests := []struct {
		name       string
		returnErr  bool
		errKey     string
		currentNAD *cniv1.NetworkAttachmentDefinition
		currentVM  *kubevirtv1.VirtualMachine
		currentVmi *kubevirtv1.VirtualMachineInstance
	}{
		{
			name:      "NAD can't be deleted as it has used VMIs",
			returnErr: true,
			errKey:    testVMName,
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
			currentVmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testVMName,
					Namespace: testNamespace,
				},
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
			}, // vmi
		},
		{
			name:      "NAD can't be deleted as it has used VMs",
			returnErr: true,
			errKey:    testVMName,
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
		},
		{
			name:      "NAD can be deleted as it has no used VMs",
			returnErr: false,
			errKey:    "",
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
		},
		{
			name:      "delete nad when subnet using it throws error",
			returnErr: true,
			errKey:    "using the nad",
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testKubeOVNNadName,
					Namespace: testKubeOVNNamespace,
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testKubeOVNNadConfig,
				},
			},
		},
	}

	currentSubnet := &kubeovnv1.Subnet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSubnetName,
			Namespace: testSubnetNamespace,
		},
		Spec: kubeovnv1.SubnetSpec{
			Vpc:      "test1",
			Protocol: "IPv4",
			Provider: "vswitch1.default.ovn",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.currentNAD)
			if tc.currentNAD == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			vmCache := fakeclients.VirtualMachineCache(harvesterclientset.KubevirtV1().VirtualMachines)
			vmiCache := fakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			subnetCache := fakeclients.SubnetCache(nchclientset.KubeovnV1().Subnets)
			validator := NewNadValidator(vmCache, vmiCache, cnCache, vcCache, subnetCache, true)

			if tc.currentVM != nil {
				err := harvesterclientset.Tracker().Add(tc.currentVM)
				assert.Nil(t, err, "mock resource vm should add into fake controller tracker")
			}

			if tc.currentVmi != nil {
				err := harvesterclientset.Tracker().Add(tc.currentVmi)
				assert.Nil(t, err, "mock resource vmi should add into fake controller tracker")
			}

			if currentSubnet != nil {
				err := nchclientset.Tracker().Add(currentSubnet)
				assert.Nil(t, err, "mock resource subnet should add into fake controller tracker")
			}

			err := validator.Delete(nil, tc.currentNAD)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				if err != nil {
					assert.True(t, strings.Contains(err.Error(), tc.errKey))
				}
			}
		})
	}
}
