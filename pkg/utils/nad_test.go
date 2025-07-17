package utils

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"github.com/stretchr/testify/assert"
)

const (
	testNadConfigRoute        = "{\"mode\":\"auto\"}"
	testNadConfigRouteInvalid = "{\"mode\":\"auto\", \"unknow\"}"
	testNadConfigRouteManual  = "{\"mode\":\"manual\"}"
	testNadConfigVlan300      = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}"
	testNadConfigVlan350      = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":350,\"ipam\":{}}"
	testNadConfigVlanUntag    = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":0,\"ipam\":{}}"
	testNadConfigVlanTrunk    = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":0,\"vlanTrunk\":[{\"minID\":300,\"maxID\":320}],\"ipam\":{}}"

	testNadConfigOVN = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-ovn\",\"type\":\"kube-ovn\"}"
)

func TestL2NetConf(t *testing.T) {
	tests := []struct {
		name          string
		returnErr     bool
		errKey        string
		nad           *cniv1.NetworkAttachmentDefinition
		vlan          int
		networklabels int
		cnname        string
		ovnci         bool
	}{
		{
			name:      "Nad netconf can be decoded as l2 vlan",
			returnErr: false,
			errKey:    "",
			nad: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlan300,
				},
			},
			vlan:          300,
			networklabels: 3,
			cnname:        testCnName,
		},
		{
			name:      "Nad netconf can be decoded as vlan trunk",
			returnErr: false,
			errKey:    "",
			nad: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlanTrunk,
				},
			},
			vlan:          0,
			networklabels: 2,
			cnname:        testCnName,
		},
		{
			name:      "Nad netconf can be decoded as vlan untag",
			returnErr: false,
			errKey:    "",
			nad: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigVlanUntag,
				},
			},
			vlan:          0,
			networklabels: 2,
			cnname:        testCnName,
		},
		{
			name:      "Nad netconf can be decoded as VON",
			returnErr: false,
			errKey:    "",
			nad: &cniv1.NetworkAttachmentDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:        testNadName,
					Namespace:   testNamespace,
					Annotations: map[string]string{"test": "test"},
					Labels:      map[string]string{KeyClusterNetworkLabel: testCnName},
				},
				Spec: cniv1.NetworkAttachmentDefinitionSpec{
					Config: testNadConfigOVN,
				},
			},
			vlan:          0,
			networklabels: 2,
			cnname:        ManagementClusterNetworkName,
			ovnci:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.nad == nil {
				return
			}
			l2conf, err := DecodeNadConfigToNetConf(tc.nad)

			assert.Nil(t, err)
			assert.True(t, tc.vlan == l2conf.GetVlan())
			lbs := make(map[string]string)
			err = l2conf.SetNetworkInfoToLabels(lbs)
			assert.Nil(t, err)
			assert.True(t, tc.networklabels == len(lbs))
			cnname, err := l2conf.GetClusterNetworkName()
			assert.Nil(t, err)
			assert.True(t, tc.cnname == cnname)

			if tc.ovnci {
				assert.True(t, l2conf.IsKubeOVNCNI())
				return
			}
			assert.True(t, !l2conf.IsKubeOVNCNI())

			var nads []*nadv1.NetworkAttachmentDefinition
			nads = append(nads, tc.nad)
			vis, err := NewVlanIDSetFromNadList(nads)
			assert.Nil(t, err)
			assert.NotNil(t, vis)
		})
	}
}
