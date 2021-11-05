//go:generate go run pkg/codegen/cleanup/main.go
//go:generate /bin/rm -rf pkg/generated
//go:generate go run pkg/codegen/main.go
//go:generate /bin/bash scripts/generate-manifest

package main

import (
	"context"
	"os"

	"github.com/rancher/wrangler/pkg/leader"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/urfave/cli"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager"
)

var (
	VERSION = "v0.0.0-dev"
)

func main() {
	app := cli.NewApp()
	app.Name = "harvester-network-controller"
	app.Version = VERSION
	app.Usage = "Harvester Network Controller, to help with cluster network configuration. Options kubeconfig or masterurl are required."
	commonFlags := []cli.Flag{
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
		cli.StringFlag{
			Name:   "namespace, n",
			EnvVar: "NAMESPACE",
			Value:  "",
			Usage:  "Namespace to watch, empty means it will watch CRDs in all namespaces.",
		},
		cli.IntFlag{
			Name:   "threads, t",
			EnvVar: "THREADS",
			Value:  2,
			Usage:  "Threadiness level to set, defaults to 2.",
		},
		cli.StringFlag{
			Name:   "management-network-type",
			EnvVar: "MANAGEMENT_NETWORK_TYPE",
			Value:  "flannel",
			Usage:  "The type of management network, defaults to flannel",
		},
		cli.StringFlag{
			Name:   "management-network-device",
			EnvVar: "MANAGEMENT_NETWORK_DEVICE",
			Value:  "flannel.1",
			Usage:  "The device of management network, such as VTEP device flannel.1 of flannel or canal, defaults to flannel.1.",
		},
		cli.BoolFlag{
			Name:   "enable-vip-controller",
			Usage:  "The bool flag to enable the vip controller in the manager network controller",
			EnvVar: "ENABLE_VIP_CONTROLLER",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:   "manager",
			Usage:  "Run manager",
			Action: managerRun,
			Flags:  commonFlags,
		},
		{
			Name:   "agent",
			Usage:  "Run agent",
			Action: agentRun,
			Flags:  commonFlags,
		},
	}

	if err := app.Run(os.Args); err != nil {
		klog.Fatal(err)
	}
}

func run(c *cli.Context, registerFuncList []config.RegisterFunc, leaderelection bool) error {
	masterURL := c.String("master")
	kubeconfig := c.String("kubeconfig")
	namespace := c.String("namespace")
	threadiness := c.Int("threads")
	mgmtNetworkType := c.String("management-network-type")
	mgmtNetworkDevice := c.String("management-network-device")

	if threadiness <= 0 {
		klog.Infof("Can not start with thread count of %d, please pass a proper thread count.", threadiness)
		return nil
	}

	klog.Infof("Starting network controller with %d threads.", threadiness)

	if namespace == "" {
		klog.Info("Starting network controller with no namespace.")
	} else {
		klog.Infof("Starting network controller in namespace: %s.", namespace)
	}

	ctx := signals.SetupSignalHandler(context.Background())

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building config from flags: %s", err.Error())
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error get client from kubeconfig: %s", err.Error())
	}

	options := &config.Options{
		MgmtNetworkType:   mgmtNetworkType,
		MgmtNetworkDevice: mgmtNetworkDevice,
	}

	management, err := config.SetupManagement(ctx, cfg, options)
	if err != nil {
		klog.Fatalf("Error building harvester controllers: %s", err.Error())
	}

	callback := func(ctx context.Context) {
		if err := management.Register(ctx, cfg, registerFuncList); err != nil {
			panic(err)
		}

		if err := management.Start(threadiness); err != nil {
			panic(err)
		}

		<-ctx.Done()
	}

	if leaderelection {
		leader.RunOrDie(ctx, "kube-system", "harvester-network-controllers", client, callback)
	} else {
		callback(ctx)
	}

	return nil
}

func managerRun(c *cli.Context) error {
	return run(c, manager.GetRegisterFuncList(c), true)
}

func agentRun(c *cli.Context) error {
	return run(c, agent.GetRegisterFuncList(), false)
}
