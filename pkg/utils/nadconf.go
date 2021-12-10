package utils

import (
	"encoding/json"
	"fmt"
	"net"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

const (
	KeyNetworkConf = networkv1.GroupName + "/route"
	KeyVlanLabel   = networkv1.GroupName + "/vlan-id"
)

type Connectivity string

const (
	Connectable   Connectivity = "true"
	Unconnectable Connectivity = "false"
	DHCPFailed    Connectivity = "DHCP failed"
	PingFailed    Connectivity = "ping failed"
)

type Mode string

const (
	Auto   Mode = "auto"
	Manual Mode = "manual"
)

type NadSelectedNetworks []nadv1.NetworkSelectionElement

type Layer3NetworkConf struct {
	Mode         Mode         `json:"mode,omitempty"`
	CIDR         string       `json:"cidr,omitempty"`
	Gateway      string       `json:"gateway,omitempty"`
	ServerIPAddr string       `json:"serverIPAddr,omitempty"`
	Connectivity Connectivity `json:"connectivity,omitempty"`
}

func NewLayer3NetworkConf(conf string) (*Layer3NetworkConf, error) {
	networkConf := &Layer3NetworkConf{}

	if err := json.Unmarshal([]byte(conf), networkConf); err != nil {
		return nil, err
	}

	// validate
	if networkConf.Mode != "" && networkConf.Mode != Auto && networkConf.Mode != Manual {
		return nil, fmt.Errorf("unknown mode %s", networkConf.Mode)
	}
	if _, _, err := net.ParseCIDR(networkConf.CIDR); networkConf.CIDR != "" && err != nil {
		return nil, fmt.Errorf("invalid CIDR %s", networkConf.CIDR)
	}
	if networkConf.Gateway != "" && net.ParseIP(networkConf.Gateway) == nil {
		return nil, fmt.Errorf("invalid gateway %s", networkConf.Gateway)
	}

	return networkConf, nil
}

func (c *Layer3NetworkConf) ToString() (string, error) {
	bytes, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func NewNADSelectedNetworks(conf string) (NadSelectedNetworks, error) {
	networks := make([]nadv1.NetworkSelectionElement, 1)
	if err := json.Unmarshal([]byte(conf), &networks); err != nil {
		return nil, err
	}

	return networks, nil
}

func (n NadSelectedNetworks) ToString() (string, error) {
	bytes, err := json.Marshal(n)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
