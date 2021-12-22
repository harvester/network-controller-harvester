package manager

import (
	"github.com/urfave/cli"

	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/clusternetwork"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/nad"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/node"
)

var registerFuncList = []config.RegisterFunc{
	clusternetwork.Register,
	node.Register,
	nad.Register,
}

func GetRegisterFuncList(c *cli.Context) []config.RegisterFunc {
	return registerFuncList
}
