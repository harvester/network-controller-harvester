package iface

const MaxDeviceNameLen = 15

func GenerateName(prefix, suffix string) string {
	maxPrefixLen := MaxDeviceNameLen - len(suffix)
	if len(prefix) > maxPrefixLen {
		return prefix[:maxPrefixLen] + suffix
	}
	return prefix + suffix
}
