package main

import (
	"context"
	"fmt"
	"os"

	ctlcni "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirt "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io"
	ctlkubevirtv1 "github.com/harvester/harvester/pkg/generated/controllers/kubevirt.io/v1"
	"github.com/harvester/harvester/pkg/indexeres"
	"github.com/harvester/webhook/pkg/config"
	"github.com/harvester/webhook/pkg/server"
	ctlcore "github.com/rancher/wrangler/pkg/generated/controllers/core"
	ctlcorev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	"github.com/rancher/wrangler/pkg/kubeconfig"
	"github.com/rancher/wrangler/pkg/signals"
	"github.com/rancher/wrangler/pkg/start"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"k8s.io/client-go/rest"
	kubevirtv1 "kubevirt.io/api/core/v1"

	ctlnetwork "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/webhook/clusternetwork"
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
	app.Action = func(_ *cli.Context) {
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

	webhookServer := server.NewWebhookServer(ctx, cfg, name, options)

	if err := webhookServer.RegisterMutators(
		nad.NewNadMutator(c.cnCache, c.vcCache),
		vlanconfig.NewVlanConfigMutator(c.nodeCache),
	); err != nil {
		return fmt.Errorf("failed to register mutators: %v", err)
	}

	if err := webhookServer.RegisterValidators(
		clusternetwork.NewCnValidator(c.vcCache),
		nad.NewNadValidator(c.vmiCache),
		vlanconfig.NewVlanConfigValidator(c.nadCache, c.vcCache, c.vsCache, c.cnCache, c.vmiCache),
	); err != nil {
		return fmt.Errorf("failed to register validators: %v", err)
	}

	if err := webhookServer.Start(); err != nil {
		return err
	}

	<-ctx.Done()

	return nil
}

type caches struct {
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
	vmiCache  ctlkubevirtv1.VirtualMachineInstanceCache
	vcCache   ctlnetworkv1.VlanConfigCache
	vsCache   ctlnetworkv1.VlanStatusCache
	cnCache   ctlnetworkv1.ClusterNetworkCache
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
		vmiCache:  kubevirtFactory.Kubevirt().V1().VirtualMachineInstance().Cache(),
		nadCache:  cniFactory.K8s().V1().NetworkAttachmentDefinition().Cache(),
		vcCache:   harvesterNetworkFactory.Network().V1beta1().VlanConfig().Cache(),
		vsCache:   harvesterNetworkFactory.Network().V1beta1().VlanStatus().Cache(),
		cnCache:   harvesterNetworkFactory.Network().V1beta1().ClusterNetwork().Cache(),
		nodeCache: coreFactory.Core().V1().Node().Cache(),
	}
	// Indexer must be added before starting the informer, otherwise panic `cannot add indexers to running index` happens
	c.vmiCache.AddIndexer(indexeres.VMByNetworkIndex, vmiByNetwork)

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

func vmiByNetwork(obj *kubevirtv1.VirtualMachineInstance) ([]string, error) {
	networks := obj.Spec.Networks
	networkNameList := make([]string, 0, len(networks))
	for _, network := range networks {
		if network.NetworkSource.Multus == nil {
			continue
		}
		networkNameList = append(networkNameList, network.NetworkSource.Multus.NetworkName)
	}
	return networkNameList, nil
}
