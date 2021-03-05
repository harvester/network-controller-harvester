package agent

import (
	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/agent/nad"
	"github.com/rancher/harvester-network-controller/pkg/controller/agent/nodenetwork"
)

var RegisterFuncList = []config.RegisterFunc{
	nad.Register,
	nodenetwork.Register,
}
