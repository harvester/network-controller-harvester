package clusternetwork

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	kubevirtv1 "kubevirt.io/api/core/v1"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	harvesterfake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	harvesterfakeclients "github.com/harvester/harvester/pkg/util/fakeclients"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const (
	testCnName    = "test-cn"
	testNadName   = "nad1"
	testNamespace = "test"
)

func TestCreateClusterNetwork(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		currentCN *networkv1.ClusterNetwork
		currentVC *networkv1.VlanConfig
		newCN     *networkv1.ClusterNetwork
	}{
		{
			name:      "ClusterNetwork can't be created as the name is too long",
			returnErr: true,
			errKey:    "the length of",
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "thisNameIsTooLongToBeAccept",
					Annotations: map[string]string{"test": "test"},
				},
			},
		},
		{
			name:      "ClusterNetwork can be created",
			returnErr: false,
			errKey:    "",
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
		},
		{
			name:      "ClusterNetwork can't be created as the MTU annotation is not allowed to be added by user",
			returnErr: true,
			errKey:    "can't be added",
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.newCN)
			if tc.newCN == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			nadCache := harvesterfakeclients.NetworkAttachmentDefinitionCache(harvesterclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			vmiCache := harvesterfakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			// client to inject test data
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}
			validator := NewCnValidator(nadCache, vmiCache, vcCache)
			err := validator.Create(nil, tc.newCN)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}

func TestUpdateClusterNetwork(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		currentCN *networkv1.ClusterNetwork
		currentVC *networkv1.VlanConfig
		newCN     *networkv1.ClusterNetwork
	}{
		{
			name:      "ClusterNetwork can be updated",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
		},
		{
			name:      "ClusterNetwork can be changed with valid MTU annotation",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1501"},
				},
			},
		},
		{
			name:      "ClusterNetwork mgmt can't be changed as new MTU annotation is not in range",
			returnErr: true,
			errKey:    "not in range",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "20000"},
				},
			},
		},
		{
			name:      "ClusterNetwork mgmt can't be changed as new MTU annotation is invalid",
			returnErr: true,
			errKey:    "not an integer",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "abc"},
				},
			},
		},
		{
			name:      "ClusterNetwork mgmt can be changed with new valid MTU annotation",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "2000"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.newCN)
			if tc.newCN == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			nadCache := harvesterfakeclients.NetworkAttachmentDefinitionCache(harvesterclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			vmiCache := fakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}
			validator := NewCnValidator(nadCache, vmiCache, vcCache)
			err := validator.Update(nil, tc.currentCN, tc.newCN)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				// avoid panic
				if err != nil {
					assert.True(t, strings.Contains(err.Error(), tc.errKey))
				}
			}
		})
	}
}

// for the mgmt network test only
func TestUpdateMgmtClusterNetwork(t *testing.T) {
	tests := []struct {
		name       string
		returnErr  bool
		errKey     string
		currentCN  *networkv1.ClusterNetwork
		newCN      *networkv1.ClusterNetwork
		currentNAD *cniv1.NetworkAttachmentDefinition
		currentVmi *kubevirtv1.VirtualMachineInstance
	}{
		{
			name:      "ClusterNetwork mgmt can be changed as the MTU empty value equals to default value even when storagenetwork is still attached",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name: utils.ManagementClusterNetworkName,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{utils.StorageNetworkAnnotation: "true", utils.KeyClusterNetworkLabel: utils.ManagementClusterNetworkName},
				},
			},
		},
		{
			name:      "ClusterNetwork mgmt can be changed as the MTU default value equals to empty value even when storagenetwork is still attached",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name: utils.ManagementClusterNetworkName,
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:   utils.ManagementClusterNetworkName,
					Labels: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{utils.StorageNetworkAnnotation: "true", utils.KeyClusterNetworkLabel: utils.ManagementClusterNetworkName},
				},
			},
		},
		{
			name:      "ClusterNetwork mgmt can be changed as the MTU 0 value equals to default value even when storagenetwork is still attached",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "0"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{utils.StorageNetworkAnnotation: "true", utils.KeyClusterNetworkLabel: utils.ManagementClusterNetworkName},
				},
			},
		},
		{
			name:      "ClusterNetwork mgmt can't be changed as the new MTU annotation is invalid",
			returnErr: true,
			errKey:    "not an integer",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "abc"},
				},
			},
		},
		{
			name:      "ClusterNetwork mgmt can't be changed as some VMs are still attached",
			returnErr: true,
			errKey:    "following VMs",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{utils.KeyUplinkMTU: "2000"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: utils.ManagementClusterNetworkName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"nad1\",\"type\":\"bridge\",\"bridge\":\"mgmt-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
			currentVmi: &kubevirtv1.VirtualMachineInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
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
	}

	nadGvr := schema.GroupVersionResource{
		Group:    "k8s.cni.cncf.io",
		Version:  "v1",
		Resource: "network-attachment-definitions",
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.newCN)
			if tc.newCN == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			nadCache := harvesterfakeclients.NetworkAttachmentDefinitionCache(harvesterclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			vmiCache := fakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)

			// no need to call vmiCache.AddIndexer(indexeres.VMByNetworkIndex, vmiByNetwork)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			if tc.currentCN != nil {
				err := nchclientset.Tracker().Add(tc.currentCN)
				assert.Nil(t, err, "mock resource clusternetwork should add into fake controller tracker")
			}
			if tc.currentVmi != nil {
				err := harvesterclientset.Tracker().Add(tc.currentVmi)
				assert.Nil(t, err, "mock resource vmi should add into fake controller tracker")
			}
			if tc.currentNAD != nil {
				if err := harvesterclientset.Tracker().Create(nadGvr, tc.currentNAD.DeepCopy(), tc.currentNAD.Namespace); err != nil {
					t.Fatalf("failed to add nad %+v", tc.currentNAD)
				}
			}

			validator := NewCnValidator(nadCache, vmiCache, vcCache)
			err := validator.Update(nil, tc.currentCN, tc.newCN)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				// avoid panic
				if err != nil {
					assert.True(t, strings.Contains(err.Error(), tc.errKey))
				}
			}
		})
	}
}

func TestDeleteClusterNetwork(t *testing.T) {
	tests := []struct {
		name       string
		returnErr  bool
		errKey     string
		currentCN  *networkv1.ClusterNetwork // delete this one
		currentVC  *networkv1.VlanConfig
		currentNAD *cniv1.NetworkAttachmentDefinition
	}{
		{
			name:      "ClusterNetwork mgmt can't be deleted",
			returnErr: true,
			errKey:    "not allowed",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.ManagementClusterNetworkName,
					Annotations: map[string]string{"test": "test"},
				},
			},
		},
		{
			name:      "ClusterNetwork can't be deleted as it has VlanConfig",
			returnErr: true,
			errKey:    "still exist",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "VC1",
					Annotations: map[string]string{utils.KeyMatchedNodes: "[\"node1\"]"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: 1500,
						},
					},
				},
			},
		},
		{
			name:      "ClusterNetwork can't be deleted as it has nad",
			returnErr: true,
			errKey:    "nads",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Labels:    map[string]string{utils.KeyClusterNetworkLabel: testCnName}, // attached to current cn
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"nad1\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "ClusterNetwork can be deleted as it has no referred VlanConfig or nad",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "VC1",
					Annotations: map[string]string{utils.KeyMatchedNodes: "[\"node1\"]"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: "unrelated"},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: "unrelated",
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: 1500,
						},
					},
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Labels:    map[string]string{utils.KeyClusterNetworkLabel: "unrelated"},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"nad1\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
	}

	nadGvr := schema.GroupVersionResource{
		Group:    "k8s.cni.cncf.io",
		Version:  "v1",
		Resource: "network-attachment-definitions",
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.currentCN)
			if tc.currentCN == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			nadCache := harvesterfakeclients.NetworkAttachmentDefinitionCache(harvesterclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			vmiCache := harvesterfakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}

			if tc.currentNAD != nil {
				if err := harvesterclientset.Tracker().Create(nadGvr, tc.currentNAD.DeepCopy(), tc.currentNAD.Namespace); err != nil {
					t.Fatalf("failed to add nad %+v", tc.currentNAD)
				}
			}

			validator := NewCnValidator(nadCache, vmiCache, vcCache)
			err := validator.Delete(nil, tc.currentCN)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				// avoid panic
				if err != nil {
					assert.True(t, strings.Contains(err.Error(), tc.errKey))
				}
			}
		})
	}
}
