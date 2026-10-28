package utils

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
)

const (
	ControllerName = "harvester-network-manager-bgp-controller"

	ConfigNamespace = "kube-system"
	ConfigName      = "harvester-bgp-config"

	SpeakerNamespace = "kube-system"
	SpeakerName      = "kube-ovn-speaker"

	ManagedByLabel = "network.harvesterhci.io/managed-by"
	ManagedByValue = "harvester-network-controller"

	HostnameLabel   = "kubernetes.io/hostname"
	OSLabel         = "kubernetes.io/os"
	BGPLabel        = "ovn.kubernetes.io/bgp"
	OVNLogName      = "kube-ovn-log"
	OVNLogMountPath = "/var/log/kube-ovn"
)

// SameSeccompProfile compares two PodSecurityContexts' SeccompProfile
// fields, treating nil contexts as distinct from non-nil ones.
func SameSeccompProfile(a, b *corev1.PodSecurityContext) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(a.SeccompProfile, b.SeccompProfile)
}

// MergeLabels returns a new map containing current's labels overlaid with
// desired's labels, so that externally-added labels on the DaemonSet are
// preserved across updates.
func MergeLabels(current, desired map[string]string) map[string]string {
	result := make(map[string]string, len(current)+len(desired))
	for k, v := range current {
		result[k] = v
	}
	for k, v := range desired {
		result[k] = v
	}
	return result
}

// BoolPtr returns a pointer to the given bool, for use in struct fields
// that require *bool (e.g. AutomountServiceAccountToken).
func BoolPtr(value bool) *bool {
	return &value
}

// HostPathDirectoryOrCreate returns a pointer to the
// HostPathDirectoryOrCreate HostPathType, used for the kube-ovn-log volume.
func HostPathDirectoryOrCreate() *corev1.HostPathType {
	hostPathType := corev1.HostPathDirectoryOrCreate
	return &hostPathType
}

// LabelsMatch reports whether every key/value in desired is present with
// the same value in current. Extra labels in current are ignored, so this
// only checks that the managed labels haven't drifted.
func LabelsMatch(current, desired map[string]string) bool {
	for key, value := range desired {
		if current[key] != value {
			return false
		}
	}
	return true
}
