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
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const (
	testSubnetName                 = "vswitch1"
	testSubnetNamespace            = "default"
	testKubeOVNNadName             = "vswitch1"
	testKubeOVNNadName2            = "vswitch2"
	testKubeOVNNamespace           = "default"
	testKubeOVNNondefaultNamespace = "nondefault"
	testKubeOVNNadConfig           = "{\"cniVersion\":\"0.3.1\",\"name\":\"vswitch1\",\"type\":\"kube-ovn\",\"server_socket\":\"/run/openvswitch/kube-ovn-daemon.sock\", \"provider\": \"vswitch1.default.ovn\"}"
	testNadConfig                  = "{\"cniVersion\":\"0.3.1\",\"name\":\"net1-vlan\",\"type\":\"bridge\",\"bridge\":\"test-cn-br\",\"promiscMode\":true,\"vlan\":300,\"ipam\":{}}"
	testNadName                    = "net1-vlan"
	testNamespace                  = "default"
	testSubnet2Name                = "subnet2"
	testKubeOVNNadConfig2          = "{\"cniVersion\":\"0.3.1\",\"name\":\"vswitch2\",\"type\":\"kube-ovn\",\"server_socket\":\"/run/openvswitch/kube-ovn-daemon.sock\", \"provider\": \"vswitch2.default.ovn\"}"
	testKubeOVNNadConfig3          = "{\"cniVersion\":\"0.3.1\",\"name\":\"vswitch2\",\"type\":\"kube-ovn\",\"server_socket\":\"/run/openvswitch/kube-ovn-daemon.sock\", \"provider\": \"vswitch2.nondefault.ovn\"}"
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
			name:      "default subnet join can be created",
			returnErr: false,
			errKey:    "",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "join",
				},
				Spec: kubeovnv1.SubnetSpec{
					Provider: "ovn",
				},
			},
		},
		{
			name:      "default subnet ovn-default can be created",
			returnErr: false,
			errKey:    "",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ovn-default",
				},
				Spec: kubeovnv1.SubnetSpec{
					Provider: "ovn",
				},
			},
		},
		{
			name:      "user created subnet with default subnets name,should fail",
			returnErr: true,
			errKey:    "not a default subnet",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ovn-default",
				},
				Spec: kubeovnv1.SubnetSpec{
					Provider: "vswitch1.default.ovn",
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
			errKey:    "not created",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:       "test",
					Protocol:  "IPv4",
					Provider:  "vswitch3.default.ovn",
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
		{
			name:      "invalid nad not of type kubeovn,return error",
			returnErr: true,
			errKey:    "network type of nad is not kubeovn",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:       "test1",
					Protocol:  "IPv4",
					Provider:  "net1-vlan.default.ovn",
					CIDRBlock: "172.20.0.0/24",
					Gateway:   "172.20.0.1",
				},
			},
		},
		{
			name:      "create subnet with provider already attached to another subnet throws error",
			returnErr: true,
			errKey:    "using the provider",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:      "test",
					Protocol: "IPv4",
					Provider: "vswitch2.default.ovn",
				},
			},
		},
		{
			name:      "create subnet with provider with same nad name but different nad namespace,should be success",
			returnErr: false,
			errKey:    "",
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:      "test",
					Protocol: "IPv4",
					Provider: "vswitch2.nondefault.ovn",
				},
			},
		},
	}

	currentNAD := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testKubeOVNNadName,
			Namespace: testKubeOVNNamespace,
			Labels:    map[string]string{utils.KeyNetworkType: "OverlayNetwork"},
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			Config: testKubeOVNNadConfig,
		},
	}

	currentNAD2 := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testKubeOVNNadName2,
			Namespace: testKubeOVNNamespace,
			Labels:    map[string]string{utils.KeyNetworkType: "OverlayNetwork"},
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			Config: testKubeOVNNadConfig2,
		},
	}

	currentNAD3 := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testKubeOVNNadName2,
			Namespace: testKubeOVNNondefaultNamespace,
			Labels:    map[string]string{utils.KeyNetworkType: "OverlayNetwork"},
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			Config: testKubeOVNNadConfig3,
		},
	}

	bridgeNAD := &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testNadName,
			Namespace:   testNamespace,
			Annotations: map[string]string{"test": "test"},
			Labels:      map[string]string{utils.KeyNetworkType: "L2VlanNetwork"},
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{
			// config is invalid
			Config: testNadConfig,
		},
	}

	oldSubnet := &kubeovnv1.Subnet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSubnet2Name,
			Namespace: testSubnetNamespace,
		},
		Spec: kubeovnv1.SubnetSpec{
			Vpc:       "test1",
			Protocol:  "IPv4",
			Provider:  "vswitch2.default.ovn",
			CIDRBlock: "172.20.11.0/24",
			Gateway:   "172.20.11.1",
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
			subnetCache := fakeclients.SubnetCache(nchclientset.KubeovnV1().Subnets)

			nadGvr := schema.GroupVersionResource{
				Group:    "k8s.cni.cncf.io",
				Version:  "v1",
				Resource: "network-attachment-definitions",
			}

			if err := harvesterclientset.Tracker().Create(nadGvr, currentNAD.DeepCopy(), currentNAD.Namespace); err != nil {
				t.Fatalf("failed to add nad %+v", currentNAD)
			}

			if err := harvesterclientset.Tracker().Create(nadGvr, currentNAD2.DeepCopy(), currentNAD2.Namespace); err != nil {
				t.Fatalf("failed to add nad %+v", currentNAD2)
			}

			if err := harvesterclientset.Tracker().Create(nadGvr, currentNAD3.DeepCopy(), currentNAD3.Namespace); err != nil {
				t.Fatalf("failed to add nad %+v", currentNAD3)
			}

			if err := harvesterclientset.Tracker().Create(nadGvr, bridgeNAD.DeepCopy(), bridgeNAD.Namespace); err != nil {
				t.Fatalf("failed to add nad %+v", bridgeNAD)
			}

			if currentVpc != nil {
				err := nchclientset.Tracker().Add(currentVpc)
				assert.Nil(t, err, "mock resource vpc should add into fake controller tracker")
			}

			if oldSubnet != nil {
				err := nchclientset.Tracker().Add(oldSubnet)
				assert.Nil(t, err, "mock resource subnet should add into fake controller tracker")
			}

			validator := NewSubnetValidator(nadCache, subnetCache, vpcCache)

			err := validator.Create(nil, tc.newSubnet)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}

func TestUpdateSubnet(t *testing.T) {
	tests := []struct {
		name      string
		returnErr bool
		errKey    string
		oldSubnet *kubeovnv1.Subnet
		newSubnet *kubeovnv1.Subnet
	}{
		{
			name:      "cannot update provider when VMs are using it, return error",
			returnErr: true,
			errKey:    "cannot update provider",
			oldSubnet: &kubeovnv1.Subnet{
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
				Status: kubeovnv1.SubnetStatus{V4UsingIPs: 3},
			},
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:       "test1",
					Protocol:  "IPv4",
					Provider:  "vswitch2.default.ovn",
					CIDRBlock: "172.20.0.0/24",
					Gateway:   "172.20.0.1",
				},
			},
		},
		{
			name:      "cannot update vpc when VMs are using it, return error",
			returnErr: true,
			errKey:    "cannot update vpc",
			oldSubnet: &kubeovnv1.Subnet{
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
				Status: kubeovnv1.SubnetStatus{V4UsingIPs: 3},
			},
			newSubnet: &kubeovnv1.Subnet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSubnetName,
					Namespace: testSubnetNamespace,
				},
				Spec: kubeovnv1.SubnetSpec{
					Vpc:       "test2",
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
			Labels:    map[string]string{utils.KeyNetworkType: "OverlayNetwork"},
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
			subnetCache := fakeclients.SubnetCache(nchclientset.KubeovnV1().Subnets)

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

			validator := NewSubnetValidator(nadCache, subnetCache, vpcCache)

			err := validator.Update(nil, tc.oldSubnet, tc.newSubnet)
			assert.True(t, tc.returnErr == (err != nil))
			if tc.returnErr {
				assert.NotNil(t, err)
				assert.True(t, strings.Contains(err.Error(), tc.errKey))
			}
		})
	}
}
