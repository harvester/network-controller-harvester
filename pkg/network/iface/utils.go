package iface

import (
	"encoding/json"

	"github.com/vishvananda/netlink"
)

func Route2String(route netlink.Route) (string, error) {
	bytes, err := json.Marshal(route)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func String2Route(route string) (*netlink.Route, error) {
	r := &netlink.Route{}

	if err := json.Unmarshal([]byte(route), r); err != nil {
		return nil, err
	}

	return r, nil
}
