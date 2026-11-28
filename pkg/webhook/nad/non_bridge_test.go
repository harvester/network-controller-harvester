package nad

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
)

const (
	testNadConfigIpvlan        = `{"cniVersion":"0.3.1","type":"ipvlan","master":"eth1","mode":"l2","ipam":{"type":"whereabouts","range":"10.99.0.0/24"}}`
	testNadConfigIpvlanChanged = `{"cniVersion":"0.3.1","type":"ipvlan","master":"eth1","mode":"l2","ipam":{"type":"whereabouts","range":"10.98.0.0/24"}}`
	testNadConfigMacvlan       = `{"cniVersion":"0.3.1","type":"macvlan","master":"eth1","mode":"bridge","ipam":{}}`
	testNadConfigHostDevice    = `{"cniVersion":"0.3.1","type":"host-device","device":"eth2","ipam":{}}`
)

func newTestNad(config string, annotations map[string]string) *cniv1.NetworkAttachmentDefinition {
	return &cniv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testNadName,
			Namespace:   testNamespace,
			Annotations: annotations,
		},
		Spec: cniv1.NetworkAttachmentDefinitionSpec{Config: config},
	}
}

func newTestValidator() *Validator {
	c := fake.NewSimpleClientset()
	return NewNadValidator(
		fakeclients.VirtualMachineCache(c.KubevirtV1().VirtualMachines),
		fakeclients.VirtualMachineInstanceCache(c.KubevirtV1().VirtualMachineInstances),
		fakeclients.ClusterNetworkCache(c.NetworkV1beta1().ClusterNetworks),
		fakeclients.VlanConfigCache(c.NetworkV1beta1().VlanConfigs),
		fakeclients.SubnetCache(c.KubeovnV1().Subnets),
		true,
		fakeclients.HostNetworkConfigCache(c.NetworkV1beta1().HostNetworkConfigs),
		fakeclients.NetworkAttachmentDefinitionCache(c.K8sCniCncfIoV1().NetworkAttachmentDefinitions),
	)
}

func newTestMutator() *Mutator {
	c := fake.NewSimpleClientset()
	return NewNadMutator(
		fakeclients.ClusterNetworkCache(c.NetworkV1beta1().ClusterNetworks),
		fakeclients.VlanConfigCache(c.NetworkV1beta1().VlanConfigs),
		fakeclients.NodeCache(c.CoreV1().Nodes),
	)
}

func TestIsHarvesterManagedCNI(t *testing.T) {
	tests := []struct {
		config string
		want   bool
	}{
		{testNadConfigVlan300, true},
		{`{"cniVersion":"0.3.1","type":"kube-ovn","provider":"nad.default.ovn"}`, true},
		{testNadConfigIpvlan, false},
		{testNadConfigMacvlan, false},
		{testNadConfigHostDevice, false},
	}
	for _, tc := range tests {
		conf, err := utils.DecodeNadConfigToNetConf(newTestNad(tc.config, nil))
		assert.NoError(t, err)
		assert.Equal(t, tc.want, conf.IsHarvesterManagedCNI(), tc.config)
	}
}

func TestValidatorNonBridgeNAD(t *testing.T) {
	v := newTestValidator()

	for _, config := range []string{testNadConfigIpvlan, testNadConfigMacvlan, testNadConfigHostDevice} {
		assert.NoError(t, v.Create(nil, newTestNad(config, nil)), "create %s", config)
	}

	// the route annotation is still validated
	err := v.Create(nil, newTestNad(testNadConfigIpvlan, map[string]string{utils.KeyNetworkRoute: testNadConfigRouteInvalid}))
	assert.Error(t, err)

	// a non-bridge NAD can be updated
	assert.NoError(t, v.Update(nil, newTestNad(testNadConfigIpvlan, nil), newTestNad(testNadConfigIpvlanChanged, nil)))

	// but its CNI type still cannot change
	err = v.Update(nil, newTestNad(testNadConfigIpvlan, nil), newTestNad(testNadConfigMacvlan, nil))
	assert.ErrorContains(t, err, "cannot be updated from network type")
}

func TestValidatorBridgeNADStillChecked(t *testing.T) {
	// no cluster network exists, so a bridge NAD must still be rejected
	err := newTestValidator().Create(nil, newTestNad(testNadConfigVlan300, nil))
	assert.Error(t, err)
}

func TestMutatorNonBridgeNAD(t *testing.T) {
	m := newTestMutator()

	patch, err := m.Create(nil, newTestNad(testNadConfigIpvlan, nil))
	assert.NoError(t, err)
	assert.Empty(t, patch)

	patch, err = m.Update(nil, newTestNad(testNadConfigIpvlan, nil), newTestNad(testNadConfigIpvlanChanged, nil))
	assert.NoError(t, err)
	assert.Empty(t, patch)
}
