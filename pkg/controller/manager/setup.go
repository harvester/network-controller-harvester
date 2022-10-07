package manager

import (
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/clusternetwork"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/nad"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/node"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/vlanconfig"
)

var RegisterFuncList = []config.RegisterFunc{
	nad.Register,
	vlanconfig.Register,
	node.Register,
	clusternetwork.Register,
}
