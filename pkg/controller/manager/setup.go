package manager

import (
	"github.com/urfave/cli"

	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/clusternetwork"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/node"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/vip"
)

var registerFuncList = []config.RegisterFunc{
	clusternetwork.Register,
	node.Register,
}

func GetRegisterFuncList(c *cli.Context) []config.RegisterFunc {
	enableVipController := c.Bool("enable-vip-controller")
	if enableVipController {
		registerFuncList = append(registerFuncList, vip.Register)
	}

	return registerFuncList
}
