package manager

import (
	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/manager/clusternetwork"
	"github.com/rancher/harvester-network-controller/pkg/controller/manager/node"
)

var RegisterFuncList = []config.RegisterFunc{
	clusternetwork.Register,
	node.Register,
}
