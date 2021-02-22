package master

import (
	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/master/node"
	"github.com/rancher/harvester-network-controller/pkg/controller/master/settings"
)

var RegisterFuncList = []config.RegisterFunc{
	node.Register,
	settings.Register,
}
