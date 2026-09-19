package nad

import (
	"encoding/json"
	"testing"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
)

// fakeNadCache is a minimal fake implementing ctlcniv1.NetworkAttachmentDefinitionCache
// sufficient for OnCNChange's List call.
type fakeNadCache struct {
	ctlcniv1.NetworkAttachmentDefinitionCache
	nads []*cniv1.NetworkAttachmentDefinition
}

func (f *fakeNadCache) List(namespace string, selector labels.Selector) ([]*cniv1.NetworkAttachmentDefinition, error) {
	var out []*cniv1.NetworkAttachmentDefinition
	for _, n := range f.nads {
		if selector.Matches(labels.Set(n.Labels)) {
			out = append(out, n)
		}
	}
	return out, nil
}

// fakeNadClient is a minimal fake implementing ctlcniv1.NetworkAttachmentDefinitionClient
// sufficient for OnCNChange's Update call.
type fakeNadClient struct {
	ctlcniv1.NetworkAttachmentDefinitionClient
	updated []*cniv1.NetworkAttachmentDefinition
}

func (f *fakeNadClient) Update(nad *cniv1.NetworkAttachmentDefinition) (*cniv1.NetworkAttachmentDefinition, error) {
	f.updated = append(f.updated, nad)
	return nad, nil
}

func TestHandler_OnCNChange(t *testing.T) {
	t.Run("nil cn", func(t *testing.T) {
		h := Handler{}
		out, err := h.OnCNChange("", nil)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("skip non-mgmt cn without mtu annotation", func(t *testing.T) {
		h := Handler{}
		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "cn-a",
				Annotations: map[string]string{},
			},
		}
		out, err := h.OnCNChange("", cn)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("invalid mtu annotation", func(t *testing.T) {
		h := Handler{}
		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cn-a",
				Annotations: map[string]string{
					utils.KeyUplinkMTU: "invalid",
				},
			},
		}
		out, err := h.OnCNChange("", cn)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("mgmt cn without uplink mtu annotation syncs nad mtu to default 1500", func(t *testing.T) {
		nad := &cniv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nad-a",
				Namespace: "default",
				Labels: map[string]string{
					utils.KeyClusterNetworkLabel: utils.ManagementClusterNetworkName,
				},
			},
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"bridge","mtu":9000}`,
			},
		}

		cache := &fakeNadCache{nads: []*cniv1.NetworkAttachmentDefinition{nad}}
		client := &fakeNadClient{}

		h := Handler{
			nadCache:  cache,
			nadClient: client,
		}

		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:        utils.ManagementClusterNetworkName,
				Annotations: map[string]string{},
			},
		}

		out, err := h.OnCNChange("", cn)
		require.NoError(t, err)
		require.Nil(t, out) // handler returns nil, nil on success

		require.Len(t, client.updated, 1)

		netConf := &utils.NetConf{}
		err = json.Unmarshal([]byte(client.updated[0].Spec.Config), netConf)
		require.NoError(t, err)
		require.Equal(t, utils.DefaultMTU, netConf.MTU)
	})

	t.Run("mgmt cn with uplink mtu annotation syncs nad mtu to given value", func(t *testing.T) {
		nad := &cniv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nad-b",
				Namespace: "default",
				Labels: map[string]string{
					utils.KeyClusterNetworkLabel: utils.ManagementClusterNetworkName,
				},
			},
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"bridge","mtu":1500}`,
			},
		}

		cache := &fakeNadCache{nads: []*cniv1.NetworkAttachmentDefinition{nad}}
		client := &fakeNadClient{}

		h := Handler{
			nadCache:  cache,
			nadClient: client,
		}

		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.ManagementClusterNetworkName,
				Annotations: map[string]string{
					utils.KeyUplinkMTU: "9000",
				},
			},
		}

		out, err := h.OnCNChange("", cn)
		require.NoError(t, err)
		require.Nil(t, out)

		require.Len(t, client.updated, 1)

		netConf := &utils.NetConf{}
		err = json.Unmarshal([]byte(client.updated[0].Spec.Config), netConf)
		require.NoError(t, err)
		require.Equal(t, 9000, netConf.MTU)
	})

	t.Run("nad already has matching mtu, no update performed", func(t *testing.T) {
		nad := &cniv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nad-c",
				Namespace: "default",
				Labels: map[string]string{
					utils.KeyClusterNetworkLabel: "cn-a",
				},
			},
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"bridge","mtu":9000}`,
			},
		}

		cache := &fakeNadCache{nads: []*cniv1.NetworkAttachmentDefinition{nad}}
		client := &fakeNadClient{}

		h := Handler{
			nadCache:  cache,
			nadClient: client,
		}

		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cn-a",
				Annotations: map[string]string{
					utils.KeyUplinkMTU: "9000",
				},
			},
		}

		out, err := h.OnCNChange("", cn)
		require.NoError(t, err)
		require.Nil(t, out)
		require.Empty(t, client.updated)
	})

	t.Run("non-mgmt cn without uplink mtu annotation does not list or update nads", func(t *testing.T) {
		nad := &cniv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nad-non-mgmt-no-mtu",
				Namespace: "default",
				Labels: map[string]string{
					utils.KeyClusterNetworkLabel: "cn-non-mgmt",
				},
			},
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"bridge","mtu":1500}`,
			},
		}

		cache := &fakeNadCache{nads: []*cniv1.NetworkAttachmentDefinition{nad}}
		client := &fakeNadClient{}
		h := Handler{
			nadCache:  cache,
			nadClient: client,
		}

		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "cn-non-mgmt",
				Annotations: map[string]string{}, // uplink-mtu not present
			},
		}

		out, err := h.OnCNChange("", cn)
		require.NoError(t, err)
		require.Nil(t, out)
		require.Empty(t, client.updated)
	})

	t.Run("non-mgmt cn with valid uplink mtu updates all related nads with different mtu", func(t *testing.T) {
		nad1 := &cniv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nad-d1",
				Namespace: "default",
				Labels: map[string]string{
					utils.KeyClusterNetworkLabel: "cn-d",
				},
			},
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"bridge","mtu":1500}`,
			},
		}
		nad2 := &cniv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nad-d2",
				Namespace: "default",
				Labels: map[string]string{
					utils.KeyClusterNetworkLabel: "cn-d",
				},
			},
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"bridge","mtu":1500}`,
			},
		}

		cache := &fakeNadCache{nads: []*cniv1.NetworkAttachmentDefinition{nad1, nad2}}
		client := &fakeNadClient{}
		h := Handler{
			nadCache:  cache,
			nadClient: client,
		}

		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cn-d",
				Annotations: map[string]string{
					utils.KeyUplinkMTU: "9000",
				},
			},
		}

		out, err := h.OnCNChange("", cn)
		require.NoError(t, err)
		require.Nil(t, out)
		require.Len(t, client.updated, 2)

		for _, updated := range client.updated {
			netConf := &utils.NetConf{}
			err = json.Unmarshal([]byte(updated.Spec.Config), netConf)
			require.NoError(t, err)
			require.Equal(t, 9000, netConf.MTU)
		}
	})

	t.Run("nad with invalid config returns error", func(t *testing.T) {
		nad := &cniv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nad-invalid-config",
				Namespace: "default",
				Labels: map[string]string{
					utils.KeyClusterNetworkLabel: "cn-e",
				},
			},
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"bridge","mtu":`, // invalid json
			},
		}

		cache := &fakeNadCache{nads: []*cniv1.NetworkAttachmentDefinition{nad}}
		client := &fakeNadClient{}
		h := Handler{
			nadCache:  cache,
			nadClient: client,
		}

		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cn-e",
				Annotations: map[string]string{
					utils.KeyUplinkMTU: "9000",
				},
			},
		}

		out, err := h.OnCNChange("", cn)
		require.Error(t, err)
		require.Nil(t, out)
		require.Empty(t, client.updated)
	})
	t.Run("nad config json decode shape", func(t *testing.T) {
		nad := &cniv1.NetworkAttachmentDefinition{
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"bridge","mtu":1500}`,
			},
		}
		netConf := &utils.NetConf{}
		err := json.Unmarshal([]byte(nad.Spec.Config), netConf)
		require.NoError(t, err)
	})
	t.Run("overlay nad is ignored for mtu sync", func(t *testing.T) {
		overlayNAD := &cniv1.NetworkAttachmentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nad-overlay",
				Namespace: "default",
				Labels: map[string]string{
					utils.KeyClusterNetworkLabel: "mgmt", utils.KeyNetworkType: "OverlayNetwork",
				},
			},
			Spec: cniv1.NetworkAttachmentDefinitionSpec{
				Config: `{"cniVersion":"0.3.1","type":"kube-ovn","mtu":1450}`,
			},
		}

		cache := &fakeNadCache{nads: []*cniv1.NetworkAttachmentDefinition{overlayNAD}}
		client := &fakeNadClient{}
		h := Handler{
			nadCache:  cache,
			nadClient: client,
		}

		cn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: "mgmt",
				Annotations: map[string]string{
					utils.KeyUplinkMTU: "9000",
				},
			},
		}

		out, err := h.OnCNChange("", cn)
		require.NoError(t, err)
		require.Nil(t, out)
		require.Empty(t, client.updated)
	})
}
