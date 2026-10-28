package bgp

import (
	"context"
	"testing"

	"github.com/harvester/harvester-network-controller/pkg/utils"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func validBGPConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ConfigName,
			Namespace: utils.ConfigNamespace,
		},
		Data: map[string]string{
			"enabled":          "true",
			"image":            "docker.io/kubeovn/kube-ovn:v1.16.1",
			"neighbor-address": "10.115.13.86",
			"neighbor-as":      "65030",
			"cluster-as":       "65000",
		},
	}
}

func validBGPConfig() bgpConfig {
	return bgpConfig{
		Enabled:         true,
		Image:           "docker.io/kubeovn/kube-ovn:v1.16.1",
		NeighborAddress: "10.115.13.86",
		NeighborAS:      65030,
		ClusterAS:       65000,
	}
}

func newTestHandler() (*Handler, *fake.Clientset) {
	client := fake.NewSimpleClientset()

	return &Handler{
		ctx:    context.Background(),
		client: client,
	}, client
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*corev1.ConfigMap)
		wantErr string
		enabled bool
	}{
		{
			name:    "valid enabled configuration",
			enabled: true,
		},
		{
			name: "valid disabled configuration",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["enabled"] = "false"
			},
		},
		{
			name: "missing enabled",
			mutate: func(cm *corev1.ConfigMap) {
				delete(cm.Data, "enabled")
			},
			wantErr: "enabled",
		},
		{
			name: "invalid enabled",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["enabled"] = "invalid"
			},
			wantErr: "enabled",
		},
		{
			name: "missing image",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["image"] = ""
			},
			wantErr: "image",
		},
		{
			name: "image contains whitespace",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["image"] = "kubeovn/image with-space"
			},
			wantErr: "image",
		},
		{
			name: "missing neighbor address",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["neighbor-address"] = ""
			},
			wantErr: "neighbor-address",
		},
		{
			name: "invalid neighbor address",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["neighbor-address"] = "not-an-ip"
			},
			wantErr: "neighbor-address",
		},
		{
			name: "ipv6 neighbor-address rejected",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["neighbor-address"] = "2001:db8::1"
			},
			wantErr: "not supported",
		},
		{
			name: "missing neighbor AS",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["neighbor-as"] = ""
			},
			wantErr: "neighbor-as",
		},
		{
			name: "invalid neighbor AS",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["neighbor-as"] = "abc"
			},
			wantErr: "neighbor-as",
		},
		{
			name: "zero neighbor AS",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["neighbor-as"] = "0"
			},
			wantErr: "neighbor-as",
		},
		{
			name: "AS exceeds uint32",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["cluster-as"] = "4294967296"
			},
			wantErr: "cluster-as",
		},
		{
			name: "missing cluster AS",
			mutate: func(cm *corev1.ConfigMap) {
				cm.Data["cluster-as"] = ""
			},
			wantErr: "cluster-as",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := validBGPConfigMap()
			if tt.mutate != nil {
				tt.mutate(cm)
			}

			cfg, err := parseConfig(cm)

			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.enabled, cfg.Enabled)

			if tt.enabled {
				require.Equal(t, "docker.io/kubeovn/kube-ovn:v1.16.1", cfg.Image)
				require.Equal(t, "10.115.13.86", cfg.NeighborAddress)
				require.Equal(t, uint32(65030), cfg.NeighborAS)
				require.Equal(t, uint32(65000), cfg.ClusterAS)
			}
		})
	}
}

func TestParseConfig_Nil(t *testing.T) {
	_, err := parseConfig(nil)
	require.Error(t, err)
}

func TestDesiredDaemonSet(t *testing.T) {
	cfg := validBGPConfig()
	ds := desiredDaemonSet(cfg)

	require.Equal(t, utils.SpeakerName, ds.Name)
	require.Equal(t, utils.SpeakerNamespace, ds.Namespace)
	require.Equal(t, utils.ManagedByValue, ds.Labels[utils.ManagedByLabel])

	require.Equal(t, "kube-ovn-speaker", ds.Spec.Template.Spec.Containers[0].Name)
	require.Equal(t, cfg.Image, ds.Spec.Template.Spec.Containers[0].Image)

	args := ds.Spec.Template.Spec.Containers[0].Args
	require.Contains(t, args, "--neighbor-address=10.115.13.86")
	require.Contains(t, args, "--neighbor-as=65030")
	require.Contains(t, args, "--cluster-as=65000")

	require.True(t, ds.Spec.Template.Spec.HostNetwork)
	require.Equal(t, "ovn", ds.Spec.Template.Spec.ServiceAccountName)
	require.Equal(t, utils.SpeakerName, ds.Spec.Template.Labels["app"])
}

func TestSpeakerNeedsUpdate(t *testing.T) {
	cfg := validBGPConfig()
	current := desiredDaemonSet(cfg)

	tests := []struct {
		name    string
		mutate  func(*bgpConfig)
		updated bool
	}{
		{
			name:    "unchanged configuration",
			updated: false,
		},
		{
			name: "image changed",
			mutate: func(cfg *bgpConfig) {
				cfg.Image = "docker.io/kubeovn/kube-ovn:v1.16.2"
			},
			updated: true,
		},
		{
			name: "neighbor address changed",
			mutate: func(cfg *bgpConfig) {
				cfg.NeighborAddress = "10.115.13.87"
			},
			updated: true,
		},
		{
			name: "neighbor AS changed",
			mutate: func(cfg *bgpConfig) {
				cfg.NeighborAS = 65031
			},
			updated: true,
		},
		{
			name: "cluster AS changed",
			mutate: func(cfg *bgpConfig) {
				cfg.ClusterAS = 65001
			},
			updated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desiredConfig := cfg
			if tt.mutate != nil {
				tt.mutate(&desiredConfig)
			}

			desired := desiredDaemonSet(desiredConfig)
			require.Equal(t, tt.updated, speakerNeedsUpdate(current, desired))
		})
	}
}

func TestReconcile(t *testing.T) {
	t.Run("creates DaemonSet when enabled", func(t *testing.T) {
		h, client := newTestHandler()

		err := h.reconcile(validBGPConfig())
		require.NoError(t, err)

		ds, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Get(context.Background(), utils.SpeakerName, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, utils.ManagedByValue, ds.Labels[utils.ManagedByLabel])
	})

	t.Run("does nothing when configuration is unchanged", func(t *testing.T) {
		h, client := newTestHandler()
		_, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Create(context.Background(), desiredDaemonSet(validBGPConfig()), metav1.CreateOptions{})
		require.NoError(t, err)

		client.ClearActions()

		err = h.reconcile(validBGPConfig())
		require.NoError(t, err)

		for _, action := range client.Actions() {
			require.NotEqual(t, "update", action.GetVerb())
			require.NotEqual(t, "patch", action.GetVerb())
		}
	})

	t.Run("updates DaemonSet when configuration changes", func(t *testing.T) {
		h, client := newTestHandler()
		_, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Create(context.Background(), desiredDaemonSet(validBGPConfig()), metav1.CreateOptions{})
		require.NoError(t, err)

		cfg := validBGPConfig()
		cfg.Image = "docker.io/kubeovn/kube-ovn:v1.16.2"
		cfg.NeighborAddress = "10.115.13.87"
		cfg.NeighborAS = 65031
		cfg.ClusterAS = 65001

		err = h.reconcile(cfg)
		require.NoError(t, err)

		ds, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Get(context.Background(), utils.SpeakerName, metav1.GetOptions{})
		require.NoError(t, err)

		container := ds.Spec.Template.Spec.Containers[0]
		require.Equal(t, cfg.Image, container.Image)
		require.Contains(t, container.Args, "--neighbor-address=10.115.13.87")
		require.Contains(t, container.Args, "--neighbor-as=65031")
		require.Contains(t, container.Args, "--cluster-as=65001")
	})

	t.Run("deletes DaemonSet when disabled", func(t *testing.T) {
		h, client := newTestHandler()
		_, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Create(context.Background(), desiredDaemonSet(validBGPConfig()), metav1.CreateOptions{})
		require.NoError(t, err)

		cfg := validBGPConfig()
		cfg.Enabled = false

		err = h.reconcile(cfg)
		require.NoError(t, err)

		_, err = client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Get(context.Background(), utils.SpeakerName, metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err))
	})

	t.Run("disabled and DaemonSet absent", func(t *testing.T) {
		h, _ := newTestHandler()

		cfg := validBGPConfig()
		cfg.Enabled = false

		require.NoError(t, h.reconcile(cfg))
	})

	t.Run("does not modify unmanaged DaemonSet", func(t *testing.T) {
		h, client := newTestHandler()
		ds := desiredDaemonSet(validBGPConfig())
		delete(ds.Labels, utils.ManagedByLabel)

		_, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Create(context.Background(), ds, metav1.CreateOptions{})
		require.NoError(t, err)

		err = h.reconcile(validBGPConfig())
		require.Error(t, err)

		got, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Get(context.Background(), utils.SpeakerName, metav1.GetOptions{})
		require.NoError(t, err)
		require.Empty(t, got.Labels[utils.ManagedByLabel])
	})

	t.Run("does not delete unmanaged DaemonSet when disabled", func(t *testing.T) {
		h, client := newTestHandler()
		ds := desiredDaemonSet(validBGPConfig())
		delete(ds.Labels, utils.ManagedByLabel)

		_, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Create(context.Background(), ds, metav1.CreateOptions{})
		require.NoError(t, err)

		cfg := validBGPConfig()
		cfg.Enabled = false

		require.Error(t, h.reconcile(cfg))

		_, err = client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Get(context.Background(), utils.SpeakerName, metav1.GetOptions{})
		require.NoError(t, err)
	})

	t.Run("recreates missing DaemonSet", func(t *testing.T) {
		h, client := newTestHandler()

		require.NoError(t, h.reconcile(validBGPConfig()))

		ds, err := client.AppsV1().
			DaemonSets(utils.SpeakerNamespace).
			Get(context.Background(), utils.SpeakerName, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, utils.ManagedByValue, ds.Labels[utils.ManagedByLabel])
	})
}

func TestOnConfigMapChange(t *testing.T) {
	t.Run("ignores nil ConfigMap", func(t *testing.T) {
		h := Handler{}

		out, err := h.OnConfigMapChange("", nil)
		require.NoError(t, err)
		require.Nil(t, out)
	})

	t.Run("ignores unrelated ConfigMap", func(t *testing.T) {
		h := Handler{}
		cm := validBGPConfigMap()
		cm.Name = "unrelated-config"

		out, err := h.OnConfigMapChange("", cm)
		require.NoError(t, err)
		require.Equal(t, cm, out)
	})

	t.Run("rejects invalid configuration", func(t *testing.T) {
		h, _ := newTestHandler()
		cm := validBGPConfigMap()
		cm.Data["neighbor-address"] = "invalid-ip"

		out, err := h.OnConfigMapChange("", cm)

		// The implementation returns the ConfigMap and
		// return nil after recording a warning event.
		require.NoError(t, err)
		require.Equal(t, cm, out)
	})
}
