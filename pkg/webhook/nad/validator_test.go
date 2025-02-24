package nad

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"

	harvesterfake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	harvesterfakeclients "github.com/harvester/harvester/pkg/util/fakeclients"

	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const (
	testCnName    = "test-cn"
	testNADConfig = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"harvester-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}"
	testNADName   = "testNad"
	testVMName    = "vm1"
	testNamespace = "test"
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
			name:      "NAD can be created",
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
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNADConfig,
				},
			},
		},
		{
			name:      "NAD can't be created as it does not have label",
			returnErr: true,
			errKey:    "nad does not have label",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"harvester-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
				},
			},
		},
		{
			name:      "NAD can't be created as it refers to none-existing cluster network",
			returnErr: true,
			errKey:    "nad refers to none-existing cluster network",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
				},
			},
			newNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: "invalid"},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"harvester-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
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
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"harvester-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
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
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"harvester-br\",\"promiscMode\":true,\"vlan\":-1,\"ipam\":{}}",
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
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"harvester-br\",\"promiscMode\":true,\"vlan\":4095,\"ipam\":{}}",
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
					Name:        testNADName,
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
					Name:        testNADName,
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
			name:      "NAD has invalid bridge suffix",
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
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"suffix-br-\":\"a\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}",
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

			vmiCache := harvesterfakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)

			// client to inject test data
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}

			validator := NewNadValidator(vmiCache, cnCache)

			err := validator.Create(nil, tc.newNAD)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
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
		usedVMs    []*kubevirtv1.VirtualMachineInstance
	}{
		{
			name:      "NAD can't be deleted as it has used VMs",
			returnErr: true,
			errKey:    testVMName,
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNADConfig,
				},
			},
			usedVMs: []*kubevirtv1.VirtualMachineInstance{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:        testVMName,
						Namespace:   testNamespace,
						Annotations: map[string]string{"test": "test"},
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
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNADConfig,
				},
			},
		},
		{
			name:      "NAD can be deleted as it is used by storagenetwork",
			returnErr: false,
			errKey:    "",
			currentNAD: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNADName,
					Namespace:   testNamespace,
					Annotations: map[string]string{utils.StorageNetworkAnnotation: "true"},
					Labels:      map[string]string{utils.KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNADConfig,
				},
			},
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
			vmiCache := harvesterfakeclients.VirtualMachineInstanceCache(harvesterclientset.KubevirtV1().VirtualMachineInstances)
			cnCache := fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks)
			validator := NewNadValidator(vmiCache, cnCache)

			// due to fake vmiCache limitation, just test generateVmiNoneStopError() instead of Delete()
			err := validator.generateVmiNoneStopError(tc.currentNAD, tc.usedVMs)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}
