package bgp

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	ctlappsv1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/apps/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Handler watches the BGP ConfigMap and the kube-ovn-speaker DaemonSet,
// reconciling the DaemonSet's existence and spec based on the ConfigMap's
// contents.
type Handler struct {
	ctx context.Context

	configMapCache ctlcorev1.ConfigMapCache
	daemonSetCache ctlappsv1.DaemonSetCache

	client kubernetes.Interface
}

// bgpConfig holds the validated, parsed representation of the BGP
// ConfigMap's data fields.
type bgpConfig struct {
	Enabled         bool
	NeighborAddress string
	NeighborAS      uint32
	ClusterAS       uint32
	Image           string
}

// Register wires up the Handler to the ConfigMap and DaemonSet controllers
// so that changes to either trigger reconciliation of the speaker
// DaemonSet.
func Register(ctx context.Context, management *config.Management) error {
	configMaps := management.CoreFactory.Core().V1().ConfigMap()
	daemonSets := management.AppsFactory.Apps().V1().DaemonSet()

	h := &Handler{
		ctx:            ctx,
		configMapCache: configMaps.Cache(),
		daemonSetCache: daemonSets.Cache(),
		client:         management.ClientSet,
	}

	configMaps.OnChange(ctx, utils.ControllerName, h.OnConfigMapChange)
	configMaps.OnRemove(ctx, utils.ControllerName, h.OnConfigMapRemove)

	daemonSets.OnChange(ctx, utils.ControllerName, h.OnDaemonSetChange)
	daemonSets.OnRemove(ctx, utils.ControllerName, h.OnDaemonSetRemove)
	return nil
}

// OnConfigMapChange is invoked whenever the BGP ConfigMap is created or
// updated. It parses and validates the config, then reconciles the speaker
// DaemonSet to match. Invalid configuration is reported via a Warning event
// rather than reconciled, so a bad edit doesn't tear down a working setup.
func (h *Handler) OnConfigMapChange(
	_ string,
	cm *corev1.ConfigMap,
) (*corev1.ConfigMap, error) {
	// Ignore ConfigMaps other than the well-known BGP config.
	if cm == nil || cm.Namespace != utils.ConfigNamespace || cm.Name != utils.ConfigName {
		return cm, nil
	}

	cfg, err := parseConfig(cm)
	if err != nil {
		// Surface invalid configuration to the user via an event instead
		// of failing reconciliation, since the previous DaemonSet state
		// (if any) should be left untouched.
		if eventErr := h.recordConfigWarning(
			cm,
			"InvalidBGPConfig",
			err.Error(),
		); eventErr != nil {
			return cm, fmt.Errorf(
				"invalid BGP configuration: %v; failed to record warning event: %w",
				err,
				eventErr,
			)
		}

		// The warning event informs the user.
		// Do not reconcile invalid configuration.
		return cm, nil
	}

	if err := h.reconcile(cfg); err != nil {
		return cm, err
	}

	return cm, nil
}

// OnConfigMapRemove is invoked when the BGP ConfigMap is deleted. Treat
// that as "BGP disabled" and remove the speaker DaemonSet accordingly.
func (h *Handler) OnConfigMapRemove(
	_ string,
	cm *corev1.ConfigMap,
) (*corev1.ConfigMap, error) {
	if cm == nil || cm.Namespace != utils.ConfigNamespace || cm.Name != utils.ConfigName {
		return cm, nil
	}

	if err := h.reconcile(bgpConfig{Enabled: false}); err != nil {
		return cm, err
	}

	return cm, nil
}

// OnDaemonSetChange is invoked whenever the speaker DaemonSet changes
// (including external edits/drift), and also with a nil object once the
// DaemonSet is confirmed absent from the informer cache. The nil case is
// matched by key, since a nil object has no Namespace/Name fields to read.
func (h *Handler) OnDaemonSetChange(
	key string,
	ds *appsv1.DaemonSet,
) (*appsv1.DaemonSet, error) {
	if ds == nil {
		if key != utils.SpeakerNamespace+"/"+utils.SpeakerName {
			return nil, nil
		}
	} else {
		if ds.Namespace != utils.SpeakerNamespace || ds.Name != utils.SpeakerName {
			return ds, nil
		}

		// Ignore unrelated DaemonSets with the same name unless this
		// controller owns them.
		if ds.Labels[utils.ManagedByLabel] != utils.ManagedByValue {
			return ds, nil
		}
	}

	cfg, err := h.currentConfig()
	if err != nil {
		return ds, err
	}

	if err := h.reconcile(cfg); err != nil {
		return ds, err
	}

	return ds, nil
}

// OnDaemonSetRemove is invoked when the speaker DaemonSet is deleted
// externally. If BGP is still enabled according to the ConfigMap, the
// DaemonSet is recreated to maintain the desired state.
//
// Wrangler installs a finalizer before invoking OnRemove callbacks for
// every DaemonSet, not just the one this controller manages. Skip
// unrelated or unmanaged DaemonSets without calling reconcile, so their
// finalizer is released immediately instead of getting stuck on the
// "refusing to delete an unmanaged speaker DaemonSet" guard in reconcile.
func (h *Handler) OnDaemonSetRemove(
	_ string,
	ds *appsv1.DaemonSet,
) (*appsv1.DaemonSet, error) {
	if ds == nil || ds.Namespace != utils.SpeakerNamespace || ds.Name != utils.SpeakerName {
		return ds, nil
	}

	// Ignore DaemonSets with the matching name/namespace that this
	// controller doesn't own, so deleting them never gets blocked by our
	// finalizer.
	if ds.Labels[utils.ManagedByLabel] != utils.ManagedByValue {
		return ds, nil
	}

	cfg, err := h.currentConfig()
	if err != nil {
		return ds, err
	}

	if err := h.reconcile(cfg); err != nil {
		return ds, err
	}

	return ds, nil
}

// currentConfig reads and parses the BGP ConfigMap from the cache. A
// missing ConfigMap is treated as "BGP disabled" rather than an error.
func (h *Handler) currentConfig() (bgpConfig, error) {
	cm, err := h.configMapCache.Get(utils.ConfigNamespace, utils.ConfigName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return bgpConfig{Enabled: false}, nil
		}
		return bgpConfig{}, fmt.Errorf("get BGP ConfigMap from cache: %w", err)
	}

	return parseConfig(cm)
}

// parseConfig validates and converts a ConfigMap's data fields into a
// bgpConfig. When "enabled" is false, the remaining fields are not
// required and are left zero-valued.
func parseConfig(cm *corev1.ConfigMap) (bgpConfig, error) {
	if cm == nil {
		return bgpConfig{}, fmt.Errorf("BGP ConfigMap must not be nil")
	}

	cfg := bgpConfig{}

	// "enabled" is always required, regardless of its value.
	enabledValue, exists := cm.Data["enabled"]
	if !exists || strings.TrimSpace(enabledValue) == "" {
		return bgpConfig{}, fmt.Errorf(
			"ConfigMap %s/%s: data.enabled is required",
			cm.Namespace, cm.Name,
		)
	}

	enabled, err := strconv.ParseBool(strings.TrimSpace(enabledValue))
	if err != nil {
		return bgpConfig{}, fmt.Errorf(
			"ConfigMap %s/%s: data.enabled must be true or false: %w",
			cm.Namespace, cm.Name, err,
		)
	}

	cfg.Enabled = enabled

	// When disabled, the remaining fields are not required.
	if !cfg.Enabled {
		return cfg, nil
	}

	// Image must be present and must not contain whitespace (a cheap sanity
	// check against obviously malformed image references).
	cfg.Image = strings.TrimSpace(cm.Data["image"])
	if cfg.Image == "" {
		return bgpConfig{}, fmt.Errorf(
			"ConfigMap %s/%s: data.image is required when BGP is enabled",
			cm.Namespace, cm.Name,
		)
	}

	// Basic validation of the image reference.
	if strings.ContainsAny(cfg.Image, " \t\n\r") {
		return bgpConfig{}, fmt.Errorf(
			"ConfigMap %s/%s: data.image must not contain whitespace",
			cm.Namespace, cm.Name,
		)
	}

	// Validate neighbor-address is a well-formed IP (v4 or v6).
	neighborAddress := strings.TrimSpace(cm.Data["neighbor-address"])
	if neighborAddress == "" {
		return bgpConfig{}, fmt.Errorf(
			"ConfigMap %s/%s: data.neighbor-address is required when BGP is enabled",
			cm.Namespace, cm.Name,
		)
	}

	ip := net.ParseIP(neighborAddress)
	if ip == nil {
		return bgpConfig{}, fmt.Errorf(
			"ConfigMap %s/%s: data.neighbor-address %q is not a valid IP address",
			cm.Namespace, cm.Name, neighborAddress,
		)
	}
	if ip.To4() == nil {
		return bgpConfig{}, fmt.Errorf(
			"ConfigMap %s/%s: data.neighbor-address %q is an IPv6 address, which is not supported yet",
			cm.Namespace, cm.Name, neighborAddress,
		)
	}
	cfg.NeighborAddress = ip.String()

	// parseASN validates that an AS number field is present and is a
	// non-zero uint32.
	parseASN := func(key string) (uint32, error) {
		value := strings.TrimSpace(cm.Data[key])
		if value == "" {
			return 0, fmt.Errorf(
				"ConfigMap %s/%s: data.%s is required when BGP is enabled",
				cm.Namespace, cm.Name, key,
			)
		}

		asn, err := strconv.ParseUint(value, 10, 32)
		if err != nil || asn == 0 {
			return 0, fmt.Errorf(
				"ConfigMap %s/%s: data.%s must be an integer between 1 and 4294967295",
				cm.Namespace, cm.Name, key,
			)
		}

		return uint32(asn), nil
	}

	cfg.NeighborAS, err = parseASN("neighbor-as")
	if err != nil {
		return bgpConfig{}, err
	}

	cfg.ClusterAS, err = parseASN("cluster-as")
	if err != nil {
		return bgpConfig{}, err
	}

	return cfg, nil
}

// reconcile ensures the speaker DaemonSet matches the desired state implied
// by cfg:
//   - cfg.Enabled == false: delete the DaemonSet if present (no-op otherwise).
//   - cfg.Enabled == true:  create the DaemonSet if absent, or update it if
//     its managed fields differ from the desired state.
//
// In both cases, reconcile refuses to touch a DaemonSet it doesn't manage
// (identified via managedByLabel), to avoid clobbering unrelated resources
// that happen to share the same name/namespace.
func (h *Handler) reconcile(cfg bgpConfig) error {
	ctx := h.ctx
	dsClient := h.client.AppsV1().DaemonSets(utils.SpeakerNamespace)

	current, err := dsClient.Get(ctx, utils.SpeakerName, metav1.GetOptions{})

	if !cfg.Enabled {
		// BGP disabled: delete the DaemonSet if it exists and is managed by
		// this controller.
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get speaker DaemonSet: %w", err)
		}

		if current.Labels[utils.ManagedByLabel] != utils.ManagedByValue {
			return fmt.Errorf("refusing to delete an unmanaged speaker DaemonSet")
		}

		if err := dsClient.Delete(ctx, utils.SpeakerName, metav1.DeleteOptions{}); err != nil &&
			!apierrors.IsNotFound(err) {
			return fmt.Errorf("delete speaker DaemonSet: %w", err)
		}
		return nil
	}

	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("get speaker DaemonSet: %w", err)
	}

	desired := desiredDaemonSet(cfg)

	if apierrors.IsNotFound(err) {
		// BGP enabled and no DaemonSet exists yet: create it.
		if _, err := dsClient.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Another reconciliation created it. The next event will
				// compare it with the desired state.
				return nil
			}
			return fmt.Errorf("create speaker DaemonSet: %w", err)
		}
		return nil
	}

	if current.Labels[utils.ManagedByLabel] != utils.ManagedByValue {
		return fmt.Errorf("refusing to update an unmanaged speaker DaemonSet")
	}

	// Nothing changed: avoid an unnecessary API call.
	if !speakerNeedsUpdate(current, desired) &&
		utils.LabelsMatch(current.Labels, desired.Labels) {
		return nil
	}

	// Preserve unrelated fields (e.g. status, other labels/annotations) by
	// copying the current object and only overwriting the managed fields.
	updated := current.DeepCopy()
	updated.Spec = desired.Spec
	updated.Labels = utils.MergeLabels(current.Labels, desired.Labels)

	if _, err := dsClient.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update speaker DaemonSet: %w", err)
	}

	return nil
}

// desiredDaemonSet builds the full desired DaemonSet object for the given
// BGP configuration. This is the single source of truth for the speaker
// Pod's spec (image, args, resources, volumes, scheduling constraints).
func desiredDaemonSet(cfg bgpConfig) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.SpeakerName,
			Namespace: utils.SpeakerNamespace,
			Labels: map[string]string{
				utils.ManagedByLabel: utils.ManagedByValue,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": utils.SpeakerName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       utils.SpeakerName,
						"component": "network",
						"type":      "infra",
					},
				},
				Spec: corev1.PodSpec{
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
								{
									LabelSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{"app": utils.SpeakerName},
									},
									TopologyKey: utils.HostnameLabel,
								},
							},
						},
					},
					PriorityClassName:            "system-node-critical",
					ServiceAccountName:           "ovn",
					AutomountServiceAccountToken: utils.BoolPtr(true),
					HostNetwork:                  true,
					SecurityContext: &corev1.PodSecurityContext{
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					NodeSelector: map[string]string{
						utils.OSLabel:  "linux",
						utils.BGPLabel: "true",
					},
					Containers: []corev1.Container{
						{
							Name:            utils.SpeakerName,
							Image:           cfg.Image,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"/kube-ovn/kube-ovn-speaker"},
							Args:            speakerArgs(cfg),
							Env: []corev1.EnvVar{
								{
									Name: "NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											APIVersion: "v1",
											FieldPath:  "spec.nodeName",
										},
									},
								},
								{
									Name: "POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											APIVersion: "v1",
											FieldPath:  "status.podIP",
										},
									},
								},
								{
									Name: "POD_IPS",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											APIVersion: "v1",
											FieldPath:  "status.podIPs",
										},
									},
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("300Mi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      utils.OVNLogName,
									MountPath: utils.OVNLogMountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: utils.OVNLogName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: utils.OVNLogMountPath,
									Type: utils.HostPathDirectoryOrCreate(),
								},
							},
						},
					},
				},
			},
		},
	}
}

// speakerArgs builds the kube-ovn-speaker command-line arguments from the
// BGP config. Changing any of neighbor-address, neighbor-as, or cluster-as
// results in a different args slice, which triggers an update via
// speakerNeedsUpdate.
func speakerArgs(cfg bgpConfig) []string {
	return []string{
		"--logtostderr=false",
		"--alsologtostderr=true",
		"--log_file=/var/log/kube-ovn/kube-ovn-speaker.log",
		"--log_file_max_size=200",
		"--neighbor-address=" + cfg.NeighborAddress,
		"--neighbor-as=" + strconv.FormatUint(uint64(cfg.NeighborAS), 10),
		"--cluster-as=" + strconv.FormatUint(uint64(cfg.ClusterAS), 10),
	}
}

// speakerNeedsUpdate reports whether the managed fields of current differ
// from desired, meaning an Update call is required.
//
// Compare the fields controlled by this controller rather than the entire
// pod spec, because the API server defaults several pod fields (e.g.
// default RestartPolicy, DNSPolicy) that would otherwise cause false
// positives on every reconciliation.
func speakerNeedsUpdate(current, desired *appsv1.DaemonSet) bool {
	if !reflect.DeepEqual(current.Spec.Selector, desired.Spec.Selector) {
		return true
	}

	if !reflect.DeepEqual(
		current.Spec.Template.Labels,
		desired.Spec.Template.Labels,
	) {
		return true
	}

	currentPod := &current.Spec.Template.Spec
	desiredPod := &desired.Spec.Template.Spec

	if currentPod.HostNetwork != desiredPod.HostNetwork ||
		currentPod.ServiceAccountName != desiredPod.ServiceAccountName ||
		currentPod.PriorityClassName != desiredPod.PriorityClassName ||
		!reflect.DeepEqual(currentPod.AutomountServiceAccountToken, desiredPod.AutomountServiceAccountToken) ||
		!reflect.DeepEqual(currentPod.NodeSelector, desiredPod.NodeSelector) ||
		!reflect.DeepEqual(currentPod.Tolerations, desiredPod.Tolerations) ||
		!reflect.DeepEqual(currentPod.Affinity, desiredPod.Affinity) ||
		!utils.SameSeccompProfile(currentPod.SecurityContext, desiredPod.SecurityContext) ||
		!reflect.DeepEqual(currentPod.Volumes, desiredPod.Volumes) {
		return true
	}

	if len(currentPod.Containers) != 1 || len(desiredPod.Containers) != 1 {
		return true
	}

	currentContainer := currentPod.Containers[0]
	desiredContainer := desiredPod.Containers[0]

	return currentContainer.Name != desiredContainer.Name ||
		currentContainer.Image != desiredContainer.Image ||
		currentContainer.ImagePullPolicy != desiredContainer.ImagePullPolicy ||
		!reflect.DeepEqual(currentContainer.Command, desiredContainer.Command) ||
		!reflect.DeepEqual(currentContainer.Args, desiredContainer.Args) ||
		!reflect.DeepEqual(currentContainer.Env, desiredContainer.Env) ||
		!reflect.DeepEqual(currentContainer.Resources, desiredContainer.Resources) ||
		!reflect.DeepEqual(currentContainer.VolumeMounts, desiredContainer.VolumeMounts)
}

// recordConfigWarning emits a Warning Event against the given ConfigMap to
// surface configuration errors to the user (e.g. via `kubectl describe
// configmap`), since invalid configuration is intentionally not reconciled.
func (h *Handler) recordConfigWarning(
	cm *corev1.ConfigMap,
	reason string,
	message string,
) error {
	now := metav1.Now()

	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: utils.ConfigName + "-",
			Namespace:    cm.Namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Namespace:  cm.Namespace,
			Name:       cm.Name,
			UID:        cm.UID,
		},
		Type:           corev1.EventTypeWarning,
		Reason:         reason,
		Message:        message,
		Source:         corev1.EventSource{Component: "harvester-network-controller"},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}

	_, err := h.client.CoreV1().
		Events(cm.Namespace).
		Create(h.ctx, event, metav1.CreateOptions{})
	return err
}
