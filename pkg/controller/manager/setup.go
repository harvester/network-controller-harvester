package manager

import (
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/clusternetwork"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/node"
)

var RegisterFuncList = []config.RegisterFunc{
	clusternetwork.Register,
	node.Register,
}
