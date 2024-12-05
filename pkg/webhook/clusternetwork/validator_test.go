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
			name:      "ClusterNetwork name is too long",
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
			name:      "ClusterNetwork is ok to create",
			returnErr: false,
			errKey:    "",
			newCN: &networkv1.ClusterNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testCnName,
					Annotations: map[string]string{"test": "test"},
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
			name:      "ClusterNetwork mgmt is not allowed to be deleted",
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
			name:      "ClusterNetwork is not allowed to be deleted as VlanConfig is existing",
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
			name:      "ClusterNetwork is allowed to be deleted",
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
			err := validator.Delete(nil, tc.currentCN)
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}
