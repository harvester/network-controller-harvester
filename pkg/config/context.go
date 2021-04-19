package config

import (
	"context"

	cniv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/rancher/lasso/pkg/controller"
	ctlcore "github.com/rancher/wrangler-api/pkg/generated/controllers/core"
	wcrd "github.com/rancher/wrangler/pkg/crd"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/schemes"
	"github.com/rancher/wrangler/pkg/start"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"

	networkv1 "github.com/rancher/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io"
	ctlcni "github.com/rancher/harvester/pkg/generated/controllers/k8s.cni.cncf.io"
	"github.com/rancher/harvester/pkg/util/crd"
)

var (
	localSchemeBuilder = runtime.SchemeBuilder{
		networkv1.AddToScheme,
	}
	AddToScheme = localSchemeBuilder.AddToScheme
	Scheme      = runtime.NewScheme()
)

func init() {
	utilruntime.Must(AddToScheme(Scheme))
	utilruntime.Must(schemes.AddToScheme(Scheme))
}

type RegisterFunc func(context.Context, *Management) error

type Management struct {
	ctx               context.Context
	ControllerFactory controller.SharedControllerFactory

	HarvesterNetworkFactory *ctlnetworkv1.Factory

	CniFactory  *ctlcni.Factory
	CoreFactory *ctlcore.Factory

	ClientSet *kubernetes.Clientset

	starters []start.Starter
}

func (s *Management) Start(threadiness int) error {
	return start.All(s.ctx, threadiness, s.starters...)
}

func (s *Management) Register(ctx context.Context, config *rest.Config, registerFuncList []RegisterFunc) error {
	if err := createCRDsIfNotExisted(ctx, config); err != nil {
		return err
	}

	for _, f := range registerFuncList {
		if err := f(ctx, s); err != nil {
			return err
		}
	}

	return nil
}

func (s *Management) NewRecorder(componentName, namespace, nodeName string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logrus.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: s.ClientSet.CoreV1().Events(namespace)})
	return eventBroadcaster.NewRecorder(Scheme, corev1.EventSource{Component: componentName, Host: nodeName})
}

func createCRDsIfNotExisted(ctx context.Context, config *rest.Config) error {
	factory, err := crd.NewFactoryFromClient(ctx, config)
	if err != nil {
		return err
	}
	return factory.
		CreateCRDsIfNotExisted(
			crd.NonNamespacedFromGV(networkv1.SchemeGroupVersion, "NodeNetwork"),
		).
		CreateCRDsIfNotExisted(
			createNetworkAttachmentDefinitionCRD(),
		).
		Wait()
}

func createNetworkAttachmentDefinitionCRD() wcrd.CRD {
	nad := crd.FromGV(cniv1.SchemeGroupVersion, "NetworkAttachmentDefinition")
	nad.PluralName = "network-attachment-definitions"
	nad.SingularName = "network-attachment-definition"
	return nad
}

func SetupManagement(ctx context.Context, restConfig *rest.Config) (*Management, error) {
	factory, err := controller.NewSharedControllerFactoryFromConfig(restConfig, Scheme)
	if err != nil {
		return nil, err
	}

	opts := &generic.FactoryOptions{
		SharedControllerFactory: factory,
	}

	management := &Management{
		ctx: ctx,
	}

	harvesterNetwork, err := ctlnetworkv1.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.HarvesterNetworkFactory = harvesterNetwork
	management.starters = append(management.starters, harvesterNetwork)

	core, err := ctlcore.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.CoreFactory = core
	management.starters = append(management.starters, core)

	cni, err := ctlcni.NewFactoryFromConfigWithOptions(restConfig, opts)
	if err != nil {
		return nil, err
	}
	management.CniFactory = cni
	management.starters = append(management.starters, cni)

	management.ClientSet, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	return management, nil
}
