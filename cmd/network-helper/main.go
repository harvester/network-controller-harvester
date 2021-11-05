package main

import (
	"fmt"
	"os"

	ctlcni "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io"
	"github.com/urfave/cli"
	"k8s.io/klog"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/harvester/harvester-network-controller/pkg/controller/manager/nad"
	"github.com/harvester/harvester-network-controller/pkg/helper"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

func main() {
	app := cli.NewApp()
	app.Name = "network-helper"
	app.Usage = "network-helper is to help get the network information through DHCP protocol from the pod within the VLAN network"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "kubeconfig, k",
			EnvVar: "KUBECONFIG",
			Value:  "",
			Usage:  "Kubernetes config files, e.g. $HOME/.kube/config",
		},
		cli.StringFlag{
			Name:   "master, m",
			EnvVar: "MASTERURL",
			Value:  "",
			Usage:  "Kubernetes cluster master URL.",
		},
		// example: [{"interface":"net1","name":"vlan178","namespace":"default"}]
		cli.StringFlag{
			Name:   "nadnetworks, n",
			EnvVar: nad.JobEnvName,
			Value:  "",
			Usage:  "NAD network information",
		},
	}
	app.Action = func(c *cli.Context) {
		if err := run(c); err != nil {
			panic(err)
		}
	}

	if err := app.Run(os.Args); err != nil {
		klog.Error(err)
	}
}

func run(c *cli.Context) error {
	masterURL := c.String("master")
	kubeconfig := c.String("kubeconfig")
	networks := c.String("nadnetworks")

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		return fmt.Errorf("error building config from flags: %w", err)
	}
	cni, err := ctlcni.NewFactoryFromConfig(cfg)
	if err != nil {
		return err
	}

	selectedNetworks, err := utils.NewNADSelectedNetworks(networks)
	if err != nil {
		return fmt.Errorf("failed to create nad selected network, networks: %s, error: %w", networks, err)
	}
	netHelper := helper.New(cni)

	for _, selectedNetwork := range selectedNetworks {
		networkConf, err := netHelper.GetVLANLayer3Network(&selectedNetwork)
		if err != nil {
			return fmt.Errorf("failed to get vlan layer3 network, selectedNetwork: %+v, error: %w", selectedNetworks, err)
		}

		if err := netHelper.RecordToNad(&selectedNetwork, networkConf); err != nil {
			return fmt.Errorf("failed to record to nad cr, error: %w", err)
		}
	}

	return nil
}