package vlanconfig

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
	harvesterfake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	harvesterfakeclients "github.com/harvester/harvester/pkg/util/fakeclients"
)

const testCnName = "test-cn"

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
			name:      "can not create VlanConfig on mgmt network",
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
			name:      "MTU is too small",
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
					Name:        "newVC",
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.MTUMin - 1,
						},
					},
				},
			},
		},
		{
			name:      "MTU is too big",
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
					Name:        "newVC",
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.MTUMax + 1,
						},
					},
				},
			},
		},
		{
			name:      "MTU is valid",
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
					Name:        "newVC",
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: utils.MTUDefault,
						},
					},
				},
			},
		},
		{
			name:      "MTU is 0 and will fallback to default value",
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
					Name:        "newVC",
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
			name:      "VlanConfigs under one ClusterNetwork can not have different MTUs",
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
							MTU: 1500,
						},
					},
				},
			},
			newVC: &networkv1.VlanConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "newVC",
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: networkv1.VlanConfigSpec{
					ClusterNetwork: testCnName,
					Uplink: networkv1.Uplink{
						LinkAttrs: &networkv1.LinkAttrs{
							MTU: 1501,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfigs have overlaps",
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
					Name:        "newVC",
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
			validator := NewVlanConfigValidator(nadCache, vcCache, vsCache, cnCache, vmiCache)

			err := validator.Create(nil, tc.newVC)
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
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
			name:      "VlanConfig has no matched nodes",
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
							MTU: 1500,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig has matched nodes but no VlanStatus",
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
							MTU: 1500,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig has matched nodes and matched VlanStatus but no nad",
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
							MTU: 1500,
						},
					},
				},
			},
		},
		{
			name:      "VlanConfig has matched nodes and matched VlanStatus and nad but no vmi",
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
							MTU: 1500,
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
			validator := NewVlanConfigValidator(nadCache, vcCache, vsCache, cnCache, vmiCache)

			err := validator.Delete(nil, tc.currentVC)
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}

		})
	}
}
