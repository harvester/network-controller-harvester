package main

import (
	controllergen "github.com/rancher/wrangler/pkg/controller-gen"
	"github.com/rancher/wrangler/pkg/controller-gen/args"
)

func main() {
	controllergen.Run(args.Options{
		OutputPackage: "github.com/rancher/harvester-network-controller/pkg/generated",
		Boilerplate:   "hack/boilerplate.go.txt",
		Groups:        map[string]args.Group{},
	})
}
