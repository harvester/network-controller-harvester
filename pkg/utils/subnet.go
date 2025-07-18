package utils

import "fmt"

const (
	joinSubnet       = "join"
	ovnDefaultSubnet = "ovn-default"
)

func IsReservedSubnet(subnetName string, provider string) (bool, error) {
	//join and ovn-default are default subnets in ovn cluster
	if subnetName == joinSubnet || subnetName == ovnDefaultSubnet {
		if provider != ovnProvider {
			return true, fmt.Errorf("not a default subnet %s as provider is %s", subnetName, provider)
		}
		return true, nil
	}

	return false, nil
}
