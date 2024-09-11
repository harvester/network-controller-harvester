package utils

const (
	DefaultMTU = 1500
	MaxMTU     = 9000
	MinMTU     = 1280 // IPv4 does not define this; IPv6 defines 1280; Harvester adopts it for both
)
