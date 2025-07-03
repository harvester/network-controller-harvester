package vlanconfig

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
	testNadName   = "net1-vlan"
	testVMName    = "vm1"
	testNamespace = "test"
	testNewVCName = "newVC"
)

func TestCreateVlanConfig(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		currentCN *networkv1.ClusterNetwork
		currentVC *networkv1.VlanConfig
		currentVS *networkv1.VlanStatus
		newVC     *networkv1.VlanConfig
	}{
		{
			name:      "VlanConfig can't be created on mgmt network",
			returnErr: true,
			errKey:    "mgmt",
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "testOnMgmt",
					Annotations: map[string]string{"test": "test"},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: utils.ManagementClusterNetworkName,
				},
			},
		},
		{
			name:      "VlanConfig can't be created as MTU is too small",
			returnErr: true,
			errKey:    "out of range",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.MinMTU - 1,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig can't be created as MTU is too big",
			returnErr: true,
			errKey:    "out of range",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.MaxMTU + 1,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig can't be created as MTU is valid",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
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
		},
		{
			name:      "VlanConfig can be created with MTU 0 and MTU will fallback to default value",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: 0,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig can be created with empty Uplink",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
				},
			},
		},
		{
			name:      "VlanConfig can't be created as VlanConfigs under one ClusterNetwork have different MTUs",
			returnErr: true,
			errKey:    "MTU",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
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
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.DefaultMTU + 1,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig can't be created as VlanConfigs have overlaps",
			returnErr: true,
			errKey:    "overlaps",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "oldVC", // belongs to another vc
				},
			},
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
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
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			assert.NotNil(t, tc.newVC)
			if tc.newVC == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			nadCache := harvesterfakeclients.NetworkAttachmentDefinitionCache(harvesterclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			vmiCache := harvesterfakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			vsCache := fakeclients.VlanStatusCache(nchclientset.NetworkV1beta1().VlanStatuses)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)

			// client to inject test data
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			vsClient := fakeclients.VlanStatusClient(nchclientset.NetworkV1beta1().VlanStatuses)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}
			if tc.currentVS != nil {
				vsClient.Create(tc.currentVS)
			}
			validator := NewVlanConfigValidator(nadCache, vcCache, vsCache, vmiCache, cnCache)

			err := validator.Create(nil, tc.newVC)
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

func TestUpdateVlanConfig(t *testing.T) {
	tests := []struct {
		name       string
		returnErr  bool
		errKey     string
		currentCN  *networkv1.ClusterNetwork
		otherVC    *networkv1.VlanConfig // other VCs under same cluster network
		currentVS  *networkv1.VlanStatus
		oldVC      *networkv1.VlanConfig // onChange, old
		newVC      *networkv1.VlanConfig // onChange, new
		currentNAD *cniv1.NetworkAttachmentDefinition
		currentVmi *kubevirtv1.VirtualMachineInstance
	}{
		{
			name:      "VlanConfig can be updated as no erros are detected",
			returnErr: true,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyVlanConfigLabel: "others"},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "others", // belongs to another vc
				},
			},
			oldVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
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
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
					Annotations: map[string]string{utils.KeyMatchedNodes: "[\"node1\"]"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.DefaultMTU + 1,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig can be updated even when 'LinkAttrs' is removed ",
			returnErr: true,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyVlanConfigLabel: "others"},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "others", // belongs to another vc
				},
			},
			oldVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
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
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
					Annotations: map[string]string{utils.KeyMatchedNodes: "[\"node1\"]"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					// the empty pointer `.Uplink.LinkAttrs` does not cause panic
				},
			},
		},
		{
			name:      "VlanConfig can't be updated as vmi is still attached",
			returnErr: true,
			errKey:    "stopped",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyVlanConfigLabel: testNewVCName},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     testNewVCName,
				},
			},
			oldVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
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
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNewVCName,
					Annotations: map[string]string{utils.KeyMatchedNodes: "[\"node1\"]"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.DefaultMTU + 1, // update MTU, should be blocked by vmi
						},
					},
				},
			}, // vc
			currentNAD: &cniv1.NetworkAttachmentDefinition{
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
				Status: kubevirtv1.VirtualMachineInstanceStatus{
					NodeName: "node1", // vmi is on the affected node
				},
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

			assert.NotNil(t, tc.newVC)
			if tc.newVC == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			nadCache := harvesterfakeclients.NetworkAttachmentDefinitionCache(harvesterclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			vmiCache := fakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			vsCache := fakeclients.VlanStatusCache(nchclientset.NetworkV1beta1().VlanStatuses)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)

			// client to inject test data
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			vsClient := fakeclients.VlanStatusClient(nchclientset.NetworkV1beta1().VlanStatuses)

			if tc.otherVC != nil {
				vcClient.Create(tc.otherVC)
			}

			if tc.oldVC != nil {
				vcClient.Create(tc.oldVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}
			if tc.currentVS != nil {
				vsClient.Create(tc.currentVS)
			}

			if tc.currentNAD != nil {
				if err := harvesterclientset.Tracker().Create(nadGvr, tc.currentNAD.DeepCopy(), tc.currentNAD.Namespace); err != nil {
					t.Fatalf("failed to add nad %+v", tc.currentNAD)
				}
			}

			if tc.currentVmi != nil {
				err := harvesterclientset.Tracker().Add(tc.currentVmi)
				assert.Nil(t, err, "mock resource vmi should add into fake controller tracker")
			}

			validator := NewVlanConfigValidator(nadCache, vcCache, vsCache, vmiCache, cnCache)

			err := validator.Update(nil, tc.oldVC, tc.newVC)
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

func TestDeleteVlanConfig(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		currentCN *networkv1.ClusterNetwork
		currentVC *networkv1.VlanConfig // delete this one
		currentVS *networkv1.VlanStatus
	}{
		{
			name:      "VlanConfig can be deleted as it has no matched nodes",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "VC1",
				},
			},
			currentVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "VC1",
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
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
		},
		{
			name:      "VlanConfig can be deleted as has matched nodes but no VlanStatus",
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
		},
		{
			name:      "VlanConfig can be deleted as it has matched nodes and matched VlanStatus but no nad",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyVlanConfigLabel: "VC1"},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "VC1",
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
							MTU: utils.DefaultMTU,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig can be deleted as it has matched nodes and matched VlanStatus and nad but no vmi",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			currentVS: &networkv1.VlanStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:        utils.Name("", testCnName, "node1"),
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyVlanConfigLabel: "VC1"},
				},
				Status: networkv1.VlStatus{
					ClusterNetwork: testCnName,
					VlanConfig:     "VC1",
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
							MTU: utils.DefaultMTU,
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.currentVC)
			if tc.currentVC == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			nadCache := harvesterfakeclients.NetworkAttachmentDefinitionCache(harvesterclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			vmiCache := harvesterfakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			vsCache := fakeclients.VlanStatusCache(nchclientset.NetworkV1beta1().VlanStatuses)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)

			// client to inject test data
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			vsClient := fakeclients.VlanStatusClient(nchclientset.NetworkV1beta1().VlanStatuses)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}
			if tc.currentVS != nil {
				vsClient.Create(tc.currentVS)
			}
			validator := NewVlanConfigValidator(nadCache, vcCache, vsCache, vmiCache, cnCache)

			err := validator.Delete(nil, tc.currentVC)
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
