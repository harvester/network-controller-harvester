package utils

import (
	"time"
)

const (
	DefaultMTU       = 1500
	MaxMTU           = 9000
	MinMTU           = 576 // IPv4 does not define this explicitly; IPv6 defines 1280; Some protocol requires 576; hence 576 is used
	defaultNamespace = "default"

	HarvesterSystemNamespaceName = "harvester-system" // don't import harvester/pkg/util to avoid loop importing, define it directly

	EnvLogLevel = "LOGLEVEL"

	// DefaultLocalHostNetworkConfigStatusTTL is the fallback duration for local state validity.
	DefaultLocalHostNetworkConfigStatusTTL = 5 * time.Minute

	// EnvLocalHostNetworkConfigStatusTTL defines the environment variable key for configuring state TTL.
	EnvLocalHostNetworkConfigStatusTTL = "LOCAL_HOST_NETWORK_CONFIG_STATUS_TTL"
)
