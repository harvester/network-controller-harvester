package agent

import (
	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/agent/hostnetwork"
	"github.com/rancher/harvester-network-controller/pkg/controller/agent/nad"
)

var RegisterFuncList = []config.RegisterFunc{
	nad.Register,
	hostnetwork.Register,
}
