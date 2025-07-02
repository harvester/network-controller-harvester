package nad

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	harvesterfake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const (
	testNadConfigRoute        = "{\"mode\":\"auto\"}"
	testNadConfigRouteInvalid = "{\"mode\":\"auto\", \"unknow\"}"
	testNadConfigRouteManual  = "{\"mode\":\"manual\"}"
	testNadConfigVlan300      = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}"
	testNadConfigVlan350      = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":350,\"ipam\":{}}"
	testNadConfigVlanUntag    = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":0,\"ipam\":{}}"
	testNadConfigVlanTrunk    = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":0,\"vlanTrunk\":[{\"minID\":300,\"maxID\":320}],\"ipam\":{}}"
)

func TestMutatorUpdateNAD(t *testing.T) {
	tests := []struct {
		name        string
		returnErr   bool
		errKey      string
		patchLength int
		currentCN   *networkv1.ClusterNetwork
		oldNAD      *cniv1.NetworkAttachmentDefinition
		currentNAD  *cniv1.NetworkAttachmentDefinition
	}{
		{
			name:        "patch network related labels",
			returnErr:   false,
			errKey:      "",
			patchLength: 1, // label patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
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
					Config: testNadConfigVlan300,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when vid changes from 300 to 350",
			returnErr:   false,
			errKey:      "",
			patchLength: 2, // label patch, annotation patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan350,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when vid changes from 300 to 350, but route annotation is invalid",
			returnErr:   true,
			errKey:      "failed, error",
			patchLength: 2, // label patch, annotation patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRouteInvalid,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan350,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when vid changes from 300 to untag",
			returnErr:   false,
			errKey:      "",
			patchLength: 2, // label patch, annotation patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlanUntag,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when vid changes from 300 to trunk",
			returnErr:   false,
			errKey:      "",
			patchLength: 2, // label patch, annotation patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlanTrunk,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when vid changes from 300 to untag, but there is no old route annotation",
			returnErr:   false,
			errKey:      "",
			patchLength: 1, // label patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Labels:    map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlanUntag,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when vid changes from 300 to untag, but there is empty annotation",
			returnErr:   false,
			errKey:      "",
			patchLength: 1, // label patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{
						//"test": "test", // empty annotations
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlanUntag,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when vid changes and the mode is auto",
			returnErr:   false,
			errKey:      "",
			patchLength: 2, // network and annotation patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan350,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when mode changes from auto to manual",
			returnErr:   false,
			errKey:      "",
			patchLength: 2, // network and annotation patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRouteManual,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
		},
		{
			name:        "patch network related labels, and route annotations outdated when mode changes from manual to auto",
			returnErr:   false,
			errKey:      "",
			patchLength: 2, // network and annotation patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRouteManual,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRoute,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
		},
		{
			name:        "patch network related labels, when route mode does not change",
			returnErr:   false,
			errKey:      "",
			patchLength: 1, // network and annotation patch
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			oldNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRouteManual,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNadName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"test":                "test",
						utils.KeyNetworkRoute: testNadConfigRouteManual,
					},
					Labels: map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.currentNAD)
			if tc.oldNAD == nil || tc.currentNAD == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()
			//vmCache := fakeclients.VirtualMachineCache(harvesterclientset.KubevirtV1().VirtualMachines)
			//vmiCache := fakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			//subnetCache := fakeclients.SubnetCache(nchclientset.KubeovnV1().Subnets)
			mutator := NewNadMutator(cnCache, vcCache)

			nadGvr := schema.GroupVersionResource{
				Group:    "k8s.cni.cncf.io",
				Version:  "v1",
				Resource: "network-attachment-definitions",
			}

			if err := harvesterclientset.Tracker().Create(nadGvr, tc.oldNAD, tc.oldNAD.Namespace); err != nil {
				t.Fatalf("failed to add nad %+v", tc.oldNAD)
			}
			if tc.currentCN != nil {
				if _, err := cnClient.Create(tc.currentCN); err != nil {
					t.Fatalf("failed to create cluster network %+v", tc.currentCN)
				}
			}
			m, err := mutator.Update(nil, tc.oldNAD, tc.currentNAD)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				if err != nil {
					assert.True(t, strings.Contains(err.Error(), tc.errKey))
				}
				return
			}
			if tc.patchLength > 0 {
				assert.True(t, len(m) == tc.patchLength)
			}
		})
	}
}
