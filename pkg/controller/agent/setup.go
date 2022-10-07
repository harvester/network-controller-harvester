package agent

import (
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/linkmonitor"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/nad"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/vlanconfig"
)

var RegisterFuncList = []config.RegisterFunc{
	nad.Register,
	vlanconfig.Register,
	linkmonitor.Register,
}
