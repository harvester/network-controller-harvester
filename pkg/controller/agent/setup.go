package agent

import (
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/clusternetwork"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/linkmonitor"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/vlanconfig"
)

var RegisterFuncList = []config.RegisterFunc{
	vlanconfig.Register,
	linkmonitor.Register,
	clusternetwork.Register,
}
