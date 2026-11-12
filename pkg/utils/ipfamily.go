package utils

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

type IPFamilyPolicy string

const (
	// EnvIPFamilyPolicy is the environment variable propagated from Helm chart values (.global.ipFamilyPolicy).
	//
	// WARNING: Do not edit this environment variable on the network-controller deployment directly.
	// It must strictly match the underlying host-OS network stack established during cluster installation;
	// a mismatched or independent change can cause system failures or panics.
	EnvIPFamilyPolicy = "IP_FAMILY_POLICY"

	// IPFamilyPolicySingleStack matches historical/IPv4-only behavior.
	IPFamilyPolicySingleStack IPFamilyPolicy = "SingleStack"

	// IPFamilyPolicyPreferDualStack indicates the host OS is in dual-IPv4-first
	// mode with the kernel IPv6 stack enabled. Under this policy, the controller
	// runtime can safely assume the host IP stack is dual-stack (IPv4+IPv6).
	IPFamilyPolicyPreferDualStack IPFamilyPolicy = "PreferDualStack"

	// IPFamilyPolicyRequireDualStack IPFamilyPolicy = "RequireDualStack" // Not supported yet
)

// GetIPFamilyPolicy reads the IP_FAMILY_POLICY environment variable,
// validates it case-insensitively against supported policies, and defaults to SingleStack.
func GetIPFamilyPolicy() IPFamilyPolicy {
	val := os.Getenv(EnvIPFamilyPolicy)
	if val == "" {
		return IPFamilyPolicySingleStack
	}

	// Normalize by trimming spaces and converting to lowercase for robust matching
	normalized := strings.ToLower(strings.TrimSpace(val))

	switch normalized {
	case strings.ToLower(string(IPFamilyPolicySingleStack)):
		return IPFamilyPolicySingleStack
	case strings.ToLower(string(IPFamilyPolicyPreferDualStack)):
		return IPFamilyPolicyPreferDualStack
	// case strings.ToLower(string(IPFamilyPolicyRequireDualStack)):
	// 	return IPFamilyPolicyRequireDualStack
	default:
		logrus.Warnf("Unknown %s value %q, defaulting to %s", EnvIPFamilyPolicy, val, IPFamilyPolicySingleStack)
		return IPFamilyPolicySingleStack
	}
}

// A few helpers to make controller runtime code easier.
// These allow the controller runtime to decouple from internal Helm chart
// value definitions, letting business logic focus purely on the underlying
// Linux host OS network stack that the network-controller operates on top of.

// IsV4Enabled reports whether IPv4 is enabled.
// It returns true for SingleStack, PreferDualStack, or an uninitialized/empty policy (which defaults to SingleStack).
func (p IPFamilyPolicy) IsV4Enabled() bool {
	return p == "" || p == IPFamilyPolicySingleStack || p == IPFamilyPolicyPreferDualStack
}

// IsV6Enabled reports whether IPv6 is enabled (i.e., PreferDualStack).
func (p IPFamilyPolicy) IsV6Enabled() bool {
	return p == IPFamilyPolicyPreferDualStack
}

// IsDualStack reports whether the host stack is configured in dual-stack mode.
func (p IPFamilyPolicy) IsDualStack() bool {
	return p == IPFamilyPolicyPreferDualStack
}

func (p IPFamilyPolicy) String() string {
	return string(p)
}
