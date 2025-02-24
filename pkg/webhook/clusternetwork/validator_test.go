package clusternetwork

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const testCnName = "test-cn"

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
			name:      "ClusterNetwork can't be created as name is too long",
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
			name:      "ClusterNetwork can't be created as MTU label is not allowed to be added by user",
			returnErr: true,
			errKey:    "can't be added",
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:   testCnName,
					Labels: map[string]string{utils.KeyUplinkMTU: "1500"},
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
			validator := NewCnValidator(vcCache)
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
			name:      "ClusterNetwork can't be changed as MTU label is not allowed to be changed by user",
			returnErr: true,
			errKey:    "can't be changed",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:   testCnName,
					Labels: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:   testCnName,
					Labels: map[string]string{utils.KeyUplinkMTU: "1501"},
				},
			},
		},
		{
			name:      "ClusterNetwork can be changed as MTU label is allowed to be deleted by user",
			returnErr: false,
			errKey:    "",
			currentCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:   testCnName,
					Labels: map[string]string{utils.KeyUplinkMTU: "1500"},
				},
			},
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name: testCnName,
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
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}
			validator := NewCnValidator(vcCache)
			err := validator.Update(nil, tc.currentCN, tc.newCN)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}

func TestDeleteClusterNetwork(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		currentCN *networkv1.ClusterNetwork // delete this one
		currentVC *networkv1.VlanConfig
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
			name:      "ClusterNetwork can be deleted as it has no VlanConfig",
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
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.currentCN)
			if tc.currentCN == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			vcCache := fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs)
			cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
			vcClient := fakeclients.VlanConfigClient(nchclientset.NetworkV1beta1().VlanConfigs)

			if tc.currentVC != nil {
				vcClient.Create(tc.currentVC)
			}
			if tc.currentCN != nil {
				cnClient.Create(tc.currentCN)
			}
			validator := NewCnValidator(vcCache)
			err := validator.Delete(nil, tc.currentCN)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}
