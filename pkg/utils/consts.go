package utils

const (
	DefaultMTU       = 1500
	MaxMTU           = 9000
	MinMTU           = 576 // IPv4 does not define this explicitly; IPv6 defines 1280; Some protocol requires 576; hence 576 is used
	defaultNamespace = "default"

	HarvesterSystemNamespaceName = "harvester-system" // don't import harvester/pkg/util to avoid loop importing, define it directly

	// HarvesterControllerUsername is the identity of the Harvester controller, which owns the RWX-managed HostNetworkConfig
	// keep it consistent with the default HARVESTER_CONTROLLER_USER_NAME in the Harvester chart
	HarvesterControllerUsername = "system:serviceaccount:" + HarvesterSystemNamespaceName + ":harvester"

	EnvLogLevel = "LOGLEVEL"
)
