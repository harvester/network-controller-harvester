package main

import (
	"os"

	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"

	networkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/apis/network.harvester.cattle.io/v1alpha1"
)

func main() {
	os.Unsetenv("GOPATH")
	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/harvester-network-controller/pkg/generated",
		Boilerplate:   "hack/boilerplate.go.txt",
		Groups: map[string]args.Group{
			"network.harvester.cattle.io": {
				Types: []interface{}{
					networkv1alpha1.NodeNetwork{},
					networkv1alpha1.ClusterNetwork{},
				},
				GenerateTypes:   true,
				GenerateClients: true,
			},
		},
	})
}
