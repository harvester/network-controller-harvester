package network

import (
	"sync"

	"github.com/rancher/harvester-network-controller/pkg/network/monitor"
)

var watcher *monitor.Monitor
var once sync.Once

func GetWatcher() *monitor.Monitor {
	once.Do(func() {
		watcher = monitor.NewMonitor()
	})
	return watcher
}
