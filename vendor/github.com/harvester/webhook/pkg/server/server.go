package server

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/mux"
	"github.com/rancher/dynamiclistener"
	"github.com/rancher/dynamiclistener/server"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/harvester/webhook/pkg/clients"
	"github.com/harvester/webhook/pkg/config"
	"github.com/harvester/webhook/pkg/types"
)

var (
	port                = int32(443)
	validationPath      = "/v1/webhook/validation"
	mutationPath        = "/v1/webhook/mutation"
	failPolicyFail      = v1.Fail
	failPolicyIgnore    = v1.Ignore
	sideEffectClassNone = v1.SideEffectClassNone
)

// AdmissionWebhookServer for listening the AdmissionReview request sent by the apiservers
type AdmissionWebhookServer struct {
	context    context.Context
	restConfig *rest.Config
	name       string
	options    *config.Options

	admitters map[types.AdmissionType][]types.Admitter
}

// New admission webhook server
func New(ctx context.Context, restConfig *rest.Config, name string, options *config.Options) *AdmissionWebhookServer {
	return &AdmissionWebhookServer{
		context:    ctx,
		restConfig: restConfig,
		name:       name,
		options:    options,

		admitters: map[types.AdmissionType][]types.Admitter{
			types.AdmissionTypeValidation: make([]types.Admitter, 0),
			types.AdmissionTypeMutation:   make([]types.Admitter, 0),
		},
	}
}

// Start the admission webhook server.
// The server will apply the validatingwebhookconfiguration and mutatingwebhookconfiguration with cert authentication automatically.
func (s *AdmissionWebhookServer) Start() error {
	client, err := clients.New(s.restConfig)
	if err != nil {
		return err
	}

	validationHandler, validationResources, err := s.validation(s.options)
	if err != nil {
		return err
	}
	mutationHandler, mutationResources, err := s.mutation(s.options)
	if err != nil {
		return err
	}

	router := mux.NewRouter()
	router.Handle(validationPath, validationHandler)
	router.Handle(mutationPath, mutationHandler)
	if err := s.listenAndServe(client, router, validationResources, mutationResources); err != nil {
		logrus.Error(err)
		return err
	}

	if err := client.Start(s.context); err != nil {
		logrus.Error(err)
		return err
	}
	return nil
}

func (s *AdmissionWebhookServer) listenAndServe(clients *clients.Clients, handler http.Handler, validationResources []types.Resource, mutationResources []types.Resource) error {
	apply := clients.Apply.WithDynamicLookup()
	caName, certName := s.name+"-ca", s.name+"-tls"

	clients.Core.Secret().OnChange(s.context, "secrets", func(key string, secret *corev1.Secret) (*corev1.Secret, error) {
		if secret == nil || secret.Name != caName || secret.Namespace != s.options.Namespace || len(secret.Data[corev1.TLSCertKey]) == 0 {
			return nil, nil
		}
		logrus.Info("Sleeping for 15 seconds then applying webhook config")
		// Sleep here to make sure server is listening and all caches are primed
		time.Sleep(15 * time.Second)

		logrus.Debugf("Building validation rules...")
		validationRules := s.buildRules(validationResources)
		logrus.Debugf("Building mutation rules...")
		mutationRules := s.buildRules(mutationResources)

		validatingWebhookConfiguration := &v1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: s.name,
			},
			Webhooks: []v1.ValidatingWebhook{
				{
					Name: "validator." + s.options.Namespace + "." + s.name,
					ClientConfig: v1.WebhookClientConfig{
						Service: &v1.ServiceReference{
							Namespace: s.options.Namespace,
							Name:      s.name,
							Path:      &validationPath,
							Port:      &port,
						},
						CABundle: secret.Data[corev1.TLSCertKey],
					},
					Rules:                   validationRules,
					FailurePolicy:           &failPolicyFail,
					SideEffects:             &sideEffectClassNone,
					AdmissionReviewVersions: []string{"v1", "v1beta1"},
				},
			},
		}

		mutatingWebhookConfiguration := &v1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: s.name,
			},
			Webhooks: []v1.MutatingWebhook{
				{
					Name: "mutator." + s.options.Namespace + "." + s.name,
					ClientConfig: v1.WebhookClientConfig{
						Service: &v1.ServiceReference{
							Namespace: s.options.Namespace,
							Name:      s.name,
							Path:      &mutationPath,
							Port:      &port,
						},
						CABundle: secret.Data[corev1.TLSCertKey],
					},
					Rules:                   mutationRules,
					FailurePolicy:           &failPolicyIgnore,
					SideEffects:             &sideEffectClassNone,
					AdmissionReviewVersions: []string{"v1", "v1beta1"},
				},
			},
		}

		return secret, apply.WithOwner(secret).ApplyObjects(validatingWebhookConfiguration, mutatingWebhookConfiguration)
	})

	tlsName := fmt.Sprintf("%s.%s.svc", s.name, s.options.Namespace)

	return server.ListenAndServe(s.context, s.options.HTTPSListenPort, 0, handler, &server.ListenOpts{
		Secrets:       clients.Core.Secret(),
		CertNamespace: s.options.Namespace,
		CertName:      certName,
		CAName:        caName,
		TLSListenerConfig: dynamiclistener.Config{
			SANs: []string{
				tlsName,
			},
			FilterCN: dynamiclistener.OnlyAllow(tlsName),
		},
	})
}

func (s *AdmissionWebhookServer) buildRules(resources []types.Resource) []v1.RuleWithOperations {
	rules := make([]v1.RuleWithOperations, 0)
	for _, rsc := range resources {
		logrus.Debugf("Add rule for %+v", rsc)
		scope := rsc.Scope
		rules = append(rules, v1.RuleWithOperations{
			Operations: rsc.OperationTypes,
			Rule: v1.Rule{
				APIGroups:   []string{rsc.APIGroup},
				APIVersions: []string{rsc.APIVersion},
				Resources:   rsc.Names,
				Scope:       &scope,
			},
		})
	}

	return rules
}

// Register validator or mutator to the admission webhook server.
// Call it before start the admission webhook server.
func (s *AdmissionWebhookServer) Register(admitters []types.Admitter) {
	mutatorType := reflect.TypeOf((*types.ValidatorAdapter)(nil))

	for _, admitter := range admitters {
		typ := reflect.TypeOf(admitter)
		logrus.Debugf("admitter type: %s", typ.String())
		if typ == mutatorType {
			s.admitters[types.AdmissionTypeValidation] = append(s.admitters[types.AdmissionTypeValidation], admitter)
			continue
		}
		s.admitters[types.AdmissionTypeMutation] = append(s.admitters[types.AdmissionTypeMutation], admitter)
	}
}
