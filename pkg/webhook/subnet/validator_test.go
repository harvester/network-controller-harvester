package subnet

import (
	"strings"
	"testing"

	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	harvesterfake "github.com/harvester/harvester/pkg/generated/clientset/versioned/fake"
	harvesterfakeclients "github.com/harvester/harvester/pkg/util/fakeclients"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const (
	testSubnetName       = "vswitch1"
	testSubnetNamespace  = "default"
	testKubeOVNNadName   = "vswitch1"
	testKubeOVNNamespace = "default"
	testKubeOVNNadConfig = "{\"cniVersion\":\"0.3.1\",\"name\":\"vswitch1\",\"type\":\"kube-ovn\",\"server_socket\":\"/run/openvswitch/kube-ovn-daemon.sock\", \"provider\": \"vswitch1.default.ovn\"}"
)

func TestCreateSubnet(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		newSubnet *kubeovnv1.Subnet
	}{
		{
			name:      "valid subnet can be created",
			returnErr: false,
			errKey:    "",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:       "test",
					Protocol:  "IPv4",
					Provider:  "vswitch1.default.ovn",
					CIDRBlock: "172.20.0.0/24",
					Gateway:   "172.20.0.1",
				},
			},
		},
		{
			name:      "provider is empty for subnet,return error",
			returnErr: true,
			errKey:    "provider is empty",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:       "test",
					Protocol:  "IPv4",
					CIDRBlock: "172.20.0.0/24",
					Gateway:   "172.20.0.1",
				},
			},
		},
		{
			name:      "nad does not exist,return error",
			returnErr: true,
			errKey:    "nad does not exist",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:       "test",
					Protocol:  "IPv4",
					Provider:  "vswitch2.default.ovn",
					CIDRBlock: "172.20.0.0/24",
					Gateway:   "172.20.0.1",
				},
			},
		},
		{
			name:      "vpc does not exist,return error",
			returnErr: true,
			errKey:    "vpc does not exist",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:       "test1",
					Protocol:  "IPv4",
					Provider:  "vswitch1.default.ovn",
					CIDRBlock: "172.20.0.0/24",
					Gateway:   "172.20.0.1",
				},
			},
		},
	}

	currentNAD := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testKubeOVNNadName,
			Namespace: testKubeOVNNamespace,
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			Config: testKubeOVNNadConfig,
		},
	}

	currentVpc := &kubeovnv1.Vpc{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			assert.NotNil(t, tc.newSubnet)
			if tc.newSubnet == nil {
				return
			}

			nchclientset := fake.NewSimpleClientset()
			harvesterclientset := harvesterfake.NewSimpleClientset()

			nadCache := harvesterfakeclients.NetworkAttachmentDefinitionCache(harvesterclientset.K8sCniCncfIoV1().NetworkAttachmentDefinitions)
			vpcCache := fakeclients.VpcCache(nchclientset.KubeovnV1().Vpcs)

			nadGvr := schema.GroupVersionResource{
				Group:    "k8s.cni.cncf.io",
				Version:  "v1",
				Resource: "network-attachment-definitions",
			}

			if err := harvesterclientset.Tracker().Create(nadGvr, currentNAD.DeepCopy(), currentNAD.Namespace); err != nil {
				t.Fatalf("failed to add nad %+v", currentNAD)
			}

			if currentVpc != nil {
				err := nchclientset.Tracker().Add(currentVpc)
				assert.Nil(t, err, "mock resource vpc should add into fake controller tracker")
			}

			validator := NewSubnetValidator(nadCache, vpcCache)

			err := validator.Create(nil, tc.newSubnet)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}
