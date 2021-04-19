package main

import (
	"os"

	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"

	networkv1 "github.com/rancher/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/harvester-network-controller/pkg/generated",
		Boilerplate:   "hack/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"network.harvesterhci.io": {
				Types: []interface{}{
					networkv1.NodeNetwork{},
					networkv1.ClusterNetwork{},
				},
				GenerateTypes:   true,
				GenerateClients: true,
			},
		},
	})
}
