package agent

import (
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/nad"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent/nodenetwork"
)

var RegisterFuncList = []config.RegisterFunc{
	nad.Register,
	nodenetwork.Register,
}
