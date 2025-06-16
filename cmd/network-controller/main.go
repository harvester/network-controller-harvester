package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/wrangler/v3/pkg/leader"
	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/urfave/cli"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/agent"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager"
)

const (
	name               = "harvester-network-controller"
	defaultThreadCount = 2
)

var (
	VERSION = "v0.0.0-dev"
)

func main() {
	app := cli.NewApp()
	app.Name = name
	app.Version = VERSION
	app.Usage = "Harvester Network Controller, to help with cluster network configuration. Options kubeconfig or masterurl are required."
	commonFlags := []cli.Flag{
		cli.StringFlag{
			Name:   "node-name",
			EnvVar: "NODENAME",
			Value:  "",
			Usage:  "node name",
		},
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
			Value:  defaultThreadCount,
			Usage:  fmt.Sprintf("Threadiness level to set, defaults to %v.", defaultThreadCount),
		},
		cli.BoolFlag{
			Name:   "enable-vip-controller",
			Usage:  "The bool flag to enable the vip controller in the manager network controller",
			EnvVar: "ENABLE_VIP_CONTROLLER",
		},
		cli.StringFlag{
			Name:   "helper-image",
			EnvVar: "HELPER_IMAGE",
			Value:  "rancher/harvester-network-helper:master-head",
			Usage:  "The image of harvester network helper, defaults to rancher/harvester-network-helper.",
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

	klog.Infof("Starting %v version %v", app.Name, app.Version)

	if err := app.Run(os.Args); err != nil {
		klog.Fatal(err)
	}
}

func run(c *cli.Context, registerFuncList []config.RegisterFunc, leaderelection bool) error {
	masterURL := c.String("master")
	kubeconfig := c.String("kubeconfig")
	namespace := c.String("namespace")
	threadiness := c.Int("threads")
	nodeName := c.String("node-name")
	helperImage := c.String("helper-image")

	if threadiness <= 0 {
		klog.Infof("Thread count of %d is invalid, fallback to default value %v.", threadiness, defaultThreadCount)
		threadiness = defaultThreadCount
	}

	klog.Infof("namespace %v threadiness %v nodeName %v helper-image %v", namespace, threadiness, nodeName, helperImage)

	ctx := signals.SetupSignalContext()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building config from flags: %s", err.Error())
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error get client from kubeconfig: %s", err.Error())
	}

	options := &config.Options{
		Namespace:   namespace,
		NodeName:    nodeName,
		HelperImage: helperImage,
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
	return run(c, manager.RegisterFuncList, true)
}

func agentRun(c *cli.Context) error {
	return run(c, agent.RegisterFuncList, false)
}
