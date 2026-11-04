package utils

const (
	DefaultMTU       = 1500
	MaxMTU           = 9000
	MinMTU           = 576 // IPv4 does not define this explicitly; IPv6 defines 1280; Some protocol requires 576; hence 576 is used
	defaultNamespace = "default"

	HarvesterSystemNamespaceName = "harvester-system" // don't import harvester/pkg/util to avoid loop importing, define it directly

	EnvLogLevel = "LOGLEVEL"

	// RFC 8415 DUID-LLT epoch: Jan 1, 2000 00:00:00 UTC (Unix timestamp 946684800)
	DUIDLLTEPOCHSeconds = 946684800
)
