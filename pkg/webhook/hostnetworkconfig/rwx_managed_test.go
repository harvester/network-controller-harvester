package hostnetworkconfig

import (
	"testing"

	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/fake"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/harvester/harvester-network-controller/pkg/utils/fakeclients"
	webhookConfig "github.com/harvester/webhook/pkg/config"
	"github.com/harvester/webhook/pkg/server/admission"
)

const (
	rwxHNCName                   = "rwx-network"
	networkControllerUsername    = "system:serviceaccount:harvester-system:harvester-network-controller"
	adminUsername                = "system:admin"
	rwxManagedErrKey             = "managed by Harvester"
	rwxTestFinalizer             = "wrangler.cattle.io/harvester-network-hostnetworkconfig-controller"
	rwxTestDescription           = "Managed by Harvester for the rwx-network setting"
	rwxTestDescriptionByOperator = "changed by operator"
)

func newRWXTestHNC(managed bool, mutators ...func(*networkv1.HostNetworkConfig)) *networkv1.HostNetworkConfig {
	hnc := &networkv1.HostNetworkConfig{
		ObjectMeta: metav1.ObjectMeta{Name: rwxHNCName},
		Spec: networkv1.HostNetworkConfigSpec{
			Description:    rwxTestDescription,
			ClusterNetwork: utils.ManagementClusterNetworkName,
			VlanID:         100,
			Mode:           IPModeDHCP,
		},
	}
	if managed {
		hnc.Labels = map[string]string{utils.RWXNetworkManagedLabel: utils.ValueTrue}
	}
	for _, m := range mutators {
		m(hnc)
	}
	return hnc
}

func withRWXTestDescription(desc string) func(*networkv1.HostNetworkConfig) {
	return func(hnc *networkv1.HostNetworkConfig) { hnc.Spec.Description = desc }
}

func withRWXTestFinalizer(hnc *networkv1.HostNetworkConfig) {
	hnc.Finalizers = []string{rwxTestFinalizer}
}

func withRWXTestDeletionTimestamp(hnc *networkv1.HostNetworkConfig) {
	now := metav1.Now()
	hnc.DeletionTimestamp = &now
}

func newRWXTestRequest(username string) *admission.Request {
	options := &webhookConfig.Options{ControllerUsername: networkControllerUsername}
	webhookReq := &webhook.Request{AdmissionRequest: admissionv1.AdmissionRequest{UserInfo: authenticationv1.UserInfo{Username: username}}}
	return admission.NewRequest(webhookReq, options)
}

func newRWXTestValidator(t *testing.T) *Validator {
	t.Helper()
	nchclientset := fake.NewSimpleClientset()
	cnClient := fakeclients.ClusterNetworkClient(nchclientset.NetworkV1beta1().ClusterNetworks)
	_, err := cnClient.Create(&networkv1.ClusterNetwork{ObjectMeta: metav1.ObjectMeta{Name: utils.ManagementClusterNetworkName}})
	assert.NoError(t, err)

	return NewHostNetworkConfigValidator(
		nil,
		fakeclients.ClusterNetworkCache(nchclientset.NetworkV1beta1().ClusterNetworks),
		fakeclients.HostNetworkConfigCache(nchclientset.NetworkV1beta1().HostNetworkConfigs),
		fakeclients.VlanConfigCache(nchclientset.NetworkV1beta1().VlanConfigs),
		fakeclients.VlanStatusCache(nchclientset.NetworkV1beta1().VlanStatuses),
		nil,
		nil,
	)
}

func TestCreateRWXManagedHostNetworkConfig(t *testing.T) {
	tests := []struct {
		name      string
		req       *admission.Request
		hnc       *networkv1.HostNetworkConfig
		returnErr bool
	}{
		{
			name: "Harvester controller can create the RWX-managed HNC",
			req:  newRWXTestRequest(utils.HarvesterControllerUsername),
			hnc:  newRWXTestHNC(true),
		},
		{
			name: "network controller can create the RWX-managed HNC",
			req:  newRWXTestRequest(networkControllerUsername),
			hnc:  newRWXTestHNC(true),
		},
		{
			name:      "user can't create an HNC labeled as RWX-managed",
			req:       newRWXTestRequest(adminUsername),
			hnc:       newRWXTestHNC(true),
			returnErr: true,
		},
		{
			name:      "request without user info can't create an HNC labeled as RWX-managed",
			req:       nil,
			hnc:       newRWXTestHNC(true),
			returnErr: true,
		},
		{
			name: "user can create an HNC not managed by RWX",
			req:  newRWXTestRequest(adminUsername),
			hnc:  newRWXTestHNC(false),
		},
		{
			name: "HNC labeled with a non-true value is not RWX-managed",
			req:  newRWXTestRequest(adminUsername),
			hnc: newRWXTestHNC(false, func(hnc *networkv1.HostNetworkConfig) {
				hnc.Labels = map[string]string{utils.RWXNetworkManagedLabel: utils.ValueFalse}
			}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := newRWXTestValidator(t).Create(tc.req, tc.hnc)
			assertRWXManagedResult(t, tc.returnErr, err)
		})
	}
}

func TestUpdateRWXManagedHostNetworkConfig(t *testing.T) {
	tests := []struct {
		name      string
		req       *admission.Request
		oldHNC    *networkv1.HostNetworkConfig
		newHNC    *networkv1.HostNetworkConfig
		returnErr bool
	}{
		{
			name:   "Harvester controller can update the RWX-managed HNC spec",
			req:    newRWXTestRequest(utils.HarvesterControllerUsername),
			oldHNC: newRWXTestHNC(true),
			newHNC: newRWXTestHNC(true, withRWXTestDescription(rwxTestDescriptionByOperator)),
		},
		{
			name:      "user can't update the RWX-managed HNC spec",
			req:       newRWXTestRequest(adminUsername),
			oldHNC:    newRWXTestHNC(true),
			newHNC:    newRWXTestHNC(true, withRWXTestDescription(rwxTestDescriptionByOperator)),
			returnErr: true,
		},
		{
			name:      "user can't remove the RWX-managed label",
			req:       newRWXTestRequest(adminUsername),
			oldHNC:    newRWXTestHNC(true),
			newHNC:    newRWXTestHNC(false),
			returnErr: true,
		},
		{
			name:      "user can't add the RWX-managed label to an existing HNC",
			req:       newRWXTestRequest(adminUsername),
			oldHNC:    newRWXTestHNC(false),
			newHNC:    newRWXTestHNC(true),
			returnErr: true,
		},
		{
			name:      "user can't change metadata of the RWX-managed HNC",
			req:       newRWXTestRequest(adminUsername),
			oldHNC:    newRWXTestHNC(true, withRWXTestFinalizer, withRWXTestDeletionTimestamp),
			newHNC:    newRWXTestHNC(true, withRWXTestDeletionTimestamp),
			returnErr: true,
		},
		{
			name:   "network controller can remove its finalizer from the RWX-managed HNC being deleted",
			req:    newRWXTestRequest(networkControllerUsername),
			oldHNC: newRWXTestHNC(true, withRWXTestFinalizer, withRWXTestDeletionTimestamp),
			newHNC: newRWXTestHNC(true, withRWXTestDeletionTimestamp),
		},
		{
			name:   "network controller can annotate the RWX-managed HNC",
			req:    newRWXTestRequest(networkControllerUsername),
			oldHNC: newRWXTestHNC(true),
			newHNC: newRWXTestHNC(true, func(hnc *networkv1.HostNetworkConfig) {
				hnc.Annotations = map[string]string{utils.KeyMatchedNodes: "[\"node1\"]"}
			}),
		},
		{
			name:      "request without user info can't update the RWX-managed HNC",
			req:       nil,
			oldHNC:    newRWXTestHNC(true),
			newHNC:    newRWXTestHNC(true, withRWXTestDescription(rwxTestDescriptionByOperator)),
			returnErr: true,
		},
		{
			name:   "user can update an HNC not managed by RWX",
			req:    newRWXTestRequest(adminUsername),
			oldHNC: newRWXTestHNC(false),
			newHNC: newRWXTestHNC(false, withRWXTestDescription(rwxTestDescriptionByOperator)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := newRWXTestValidator(t).Update(tc.req, tc.oldHNC, tc.newHNC)
			assertRWXManagedResult(t, tc.returnErr, err)
		})
	}
}

func TestDeleteRWXManagedHostNetworkConfig(t *testing.T) {
	tests := []struct {
		name      string
		req       *admission.Request
		hnc       *networkv1.HostNetworkConfig
		returnErr bool
	}{
		{
			name: "Harvester controller can delete the RWX-managed HNC",
			req:  newRWXTestRequest(utils.HarvesterControllerUsername),
			hnc:  newRWXTestHNC(true),
		},
		{
			name:      "user can't delete the RWX-managed HNC",
			req:       newRWXTestRequest(adminUsername),
			hnc:       newRWXTestHNC(true),
			returnErr: true,
		},
		{
			name:      "request without user info can't delete the RWX-managed HNC",
			req:       nil,
			hnc:       newRWXTestHNC(true),
			returnErr: true,
		},
		{
			name: "user can delete an HNC not managed by RWX",
			req:  newRWXTestRequest(adminUsername),
			hnc:  newRWXTestHNC(false),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := newRWXTestValidator(t).Delete(tc.req, tc.hnc)
			assertRWXManagedResult(t, tc.returnErr, err)
		})
	}
}

func assertRWXManagedResult(t *testing.T, returnErr bool, err error) {
	t.Helper()
	if !returnErr {
		assert.NoError(t, err)
		return
	}
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), rwxManagedErrKey)
	}
}
