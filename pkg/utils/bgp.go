package utils

import (
	"fmt"
	"net"
	"reflect"
	"strings"

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

// ParseIPv4CSV validates a comma-separated list of IPv4 addresses found in
// cm.Data[key] and returns them as a normalized []string. IPv6 addresses
// are rejected, since BGP speaking over IPv6 is not supported yet.
func ParseIPv4CSV(namespace, name, key, raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf(
			"ConfigMap %s/%s: data.%s is required when BGP is enabled",
			namespace, name, key,
		)
	}

	parts := strings.Split(raw, ",")
	addrs := make([]string, 0, len(parts))
	for _, part := range parts {
		addr := strings.TrimSpace(part)
		if addr == "" {
			continue
		}

		ip := net.ParseIP(addr)
		if ip == nil {
			return nil, fmt.Errorf(
				"ConfigMap %s/%s: data.%s contains %q which is not a valid IP address",
				namespace, name, key, addr,
			)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf(
				"ConfigMap %s/%s: data.%s contains %q which is an IPv6 address, not supported yet",
				namespace, name, key, addr,
			)
		}

		addrs = append(addrs, ip.String())
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf(
			"ConfigMap %s/%s: data.%s must contain at least one valid IP address",
			namespace, name, key,
		)
	}

	return addrs, nil
}
