package main

import (
	"context"
	"github.com/harvester/harvester/pkg/indexeres"
	"os"

	ctlcni "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	ctlcore "github.com/rancher/wrangler/pkg/generated/controllers/core"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/rancher/wrangler/pkg/start"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"github.com/yaocw2020/webhook/pkg/config"
	"github.com/yaocw2020/webhook/pkg/server"
	"github.com/yaocw2020/webhook/pkg/types"
	"k8s.io/client-go/rest"

	ctlnetwork "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/webhook/nad"
	"github.com/harvester/harvester-network-controller/pkg/webhook/vlanconfig"
)

const name = "harvester-network-webhook"

func main() {
	var options config.Options
	var logLevel string

	flags := []cli.Flag{
		cli.StringFlag{
			Name:        "loglevel",
			Usage:       "Specify log level",
			EnvVar:      "LOGLEVEL",
			Value:       "info",
			Destination: &logLevel,
		},
		cli.IntFlag{
			Name:        "threadiness",
			EnvVar:      "THREADINESS",
			Usage:       "Specify controller threads",
			Value:       5,
			Destination: &options.Threadiness,
		},
		cli.IntFlag{
			Name:        "https-port",
			EnvVar:      "WEBHOOK_SERVER_HTTPS_PORT",
			Usage:       "HTTPS listen port",
			Value:       8443,
			Destination: &options.HTTPSListenPort,
		},
		cli.StringFlag{
			Name:        "namespace",
			EnvVar:      "NAMESPACE",
			Destination: &options.Namespace,
			Usage:       "The harvester namespace",
			Value:       "harvester-system",
		},
		cli.StringFlag{
			Name:        "controller-user",
			EnvVar:      "CONTROLLER_USER_NAME",
			Destination: &options.ControllerUsername,
			Value:       "harvester-load-balancer-webhook",
			Usage:       "The harvester controller username",
		},
		cli.StringFlag{
			Name:        "gc-user",
			EnvVar:      "GARBAGE_COLLECTION_USER_NAME",
			Destination: &options.GarbageCollectionUsername,
			Usage:       "The system username that performs garbage collection",
			Value:       "system:serviceaccount:kube-system:generic-garbage-collector",
		},
	}

	cfg, err := kubeconfig.GetNonInteractiveClientConfig(os.Getenv("KUBECONFIG")).ClientConfig()
	if err != nil {
		logrus.Fatal(err)
	}

	ctx := signals.SetupSignalContext()

	app := cli.NewApp()
	app.Flags = flags
	app.Action = func(c *cli.Context) {
		setLogLevel(logLevel)
		if err := run(ctx, cfg, &options); err != nil {
			logrus.Fatalf("run webhook server failed: %v", err)
		}
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatalf("run webhook server failed: %v", err)
	}
}

func run(ctx context.Context, cfg *rest.Config, options *config.Options) error {
	c, err := newCaches(ctx, cfg, options.Threadiness)
	if err != nil {
		return err
	}

	webhookServer := server.New(ctx, cfg, name, options)
	admitters := []types.Admitter{
		types.Validator2Admitter(nad.NewNadValidator(c.vmCache)),
		types.Validator2Admitter(vlanconfig.NewVlanConfigValidator(c.nadCache, c.vsCache)),
		nad.NewNadMutator(),
		vlanconfig.NewNadMutator(c.nodeCache),
	}
	webhookServer.Register(admitters)
	if err := webhookServer.Start(); err != nil {
		return err
	}

	<-ctx.Done()

	return nil
}

type caches struct {
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
	vmCache   ctlkubevirtv1.VirtualMachineCache
	vsCache   ctlnetworkv1.VlanStatusCache
	nodeCache ctlcorev1.NodeCache
}

func newCaches(ctx context.Context, cfg *rest.Config, threadiness int) (*caches, error) {
	var starters []start.Starter

	kubevirtFactory := ctlkubevirt.NewFactoryFromConfigOrDie(cfg)
	starters = append(starters, kubevirtFactory)
	cniFactory := ctlcni.NewFactoryFromConfigOrDie(cfg)
	starters = append(starters, cniFactory)
	harvesterNetworkFactory := ctlnetwork.NewFactoryFromConfigOrDie(cfg)
	starters = append(starters, harvesterNetworkFactory)
	coreFactory := ctlcore.NewFactoryFromConfigOrDie(cfg)
	starters = append(starters, coreFactory)
	// must declare cache before starting informers
	c := &caches{
		vmCache:   kubevirtFactory.Kubevirt().V1().VirtualMachine().Cache(),
		nadCache:  cniFactory.K8s().V1().NetworkAttachmentDefinition().Cache(),
		vsCache:   harvesterNetworkFactory.Network().V1beta1().VlanStatus().Cache(),
		nodeCache: coreFactory.Core().V1().Node().Cache(),
	}
	// Indexer must be added before starting the informer, otherwise panic `cannot add indexers to running index` happens
	c.vmCache.AddIndexer(indexeres.VMByNetworkIndex, indexeres.VMByNetwork)

	if err := start.All(ctx, threadiness, starters...); err != nil {
		return nil, err
	}

	return c, nil
}

func setLogLevel(level string) {
	ll, err := logrus.ParseLevel(level)
	if err != nil {
		ll = logrus.DebugLevel
	}
	// set global log level
	logrus.SetLevel(ll)
}
