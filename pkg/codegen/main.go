package main

import (
	"os"

	kubeovnsubnetv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	controllergen "github.com/rancher/wrangler/v3/pkg/controller-gen"
	"github.com/rancher/wrangler/v3/pkg/controller-gen/args"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/harvester/harvester-network-controller/pkg/generated",
		Boilerplate:   "hack/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"network.harvesterhci.io": {
				Types: []interface{}{
					networkv1.ClusterNetwork{},
					networkv1.VlanConfig{},
					networkv1.VlanStatus{},
					networkv1.LinkMonitor{},
				},
				GenerateTypes:   true,
				GenerateClients: true,
			},
			kubeovnsubnetv1.SchemeGroupVersion.Group: {
				Types: []interface{}{
					kubeovnsubnetv1.Subnet{},
					kubeovnsubnetv1.Vpc{},
				},
				GenerateTypes:   false,
				GenerateClients: true,
			},
		},
	})
}
