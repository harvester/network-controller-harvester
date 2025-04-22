package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rancher/dynamiclistener"
	dls "github.com/rancher/dynamiclistener/server"
	"github.com/rancher/wrangler/v3/pkg/webhook"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/harvester/webhook/pkg/clients"
	"github.com/harvester/webhook/pkg/config"
	"github.com/harvester/webhook/pkg/server/admission"
	"github.com/harvester/webhook/pkg/server/conversion"
)

var (
	port                = int32(443)
	validationPath      = "/v1/webhook/validation"
	mutationPath        = "/v1/webhook/mutation"
	conversionPath      = "/v1/webhook/conversion"
	failPolicyFail      = v1.Fail
	failPolicyIgnore    = v1.Ignore
	sideEffectClassNone = v1.SideEffectClassNone
	whiteListedCiphers  = []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}
)

type server struct {
	context    context.Context
	restConfig *rest.Config
	name       string
	options    *config.Options
	caBundle   []byte
}

// WebhookServer for listening the AdmissionReview request sent by the apiservers
type WebhookServer struct {
	server

	isStarted bool

	validators []admission.Validator
	mutators   []admission.Mutator
	converters []conversion.Converter
}

// NewWebhookServer creates a new server for admitter webhook
func NewWebhookServer(ctx context.Context, restConfig *rest.Config, name string, options *config.Options) *WebhookServer {
	return &WebhookServer{
		server: server{
			context:    ctx,
			restConfig: restConfig,
			name:       name,
			options:    options,
		},
	}
}

// Start the admitter webhook server.
// The server will apply the validatingwebhookconfiguration, mutatingwebhookconfiguration
// and CRD conversion webhook configuration with cert authentication automatically.
func (s *WebhookServer) Start() error {
	client, err := clients.New(s.restConfig)
	if err != nil {
		return err
	}

	router := mux.NewRouter()
	validatingHandler := s.validatingHandler()
	if validatingHandler != nil {
		router.Handle(validationPath, validatingHandler)
	}

	mutatingHandler := s.mutatingHandler()
	if mutatingHandler != nil {
		router.Handle(mutationPath, mutatingHandler)
	}

	if len(s.converters) != 0 {
		router.Handle(conversionPath, conversion.NewHandler(s.converters, client.RESTMapper))
	}

	if err := s.listenAndServe(client, router); err != nil {
		logrus.Errorf("listen and serve failed, err: %s", err.Error())
		return err
	}

	if err := client.Start(s.context); err != nil {
		logrus.Errorf("start clients failed, err: %s", err.Error())
		return err
	}

	s.isStarted = true

	return nil
}

func (s *WebhookServer) listenAndServe(clients *clients.Clients, handler http.Handler) error {
	apply := clients.Apply.WithDynamicLookup()
	caName, certName := s.name+"-ca", s.name+"-tls"

	clients.Core.Secret().OnChange(s.context, "secrets", func(key string, secret *corev1.Secret) (*corev1.Secret, error) {
		if secret == nil || secret.Name != caName || secret.Namespace != s.options.Namespace || len(secret.Data[corev1.TLSCertKey]) == 0 {
			return nil, nil
		}
		logrus.Info("Sleeping for 15 seconds then applying webhook config")
		// Sleep here to make sure server is listening and all caches are primed
		time.Sleep(15 * time.Second)

		s.caBundle = secret.Data[corev1.TLSCertKey]
		// configure admitter webhook
		validatingWebhookConfiguration := s.validatingWebhookConfiguration()
		mutatingWebhookConfiguration := s.mutatingWebhookConfiguration()
		if validatingWebhookConfiguration != nil || mutatingWebhookConfiguration != nil {
			if err := apply.WithOwner(secret).ApplyObjects(validatingWebhookConfiguration, mutatingWebhookConfiguration); err != nil {
				return nil, fmt.Errorf("configure validatingwebhookconfiguration %s and mutatingwebhookconfiguration %s failed, error: %w",
					validatingWebhookConfiguration.Name, mutatingWebhookConfiguration.Name, err)
			}
		}
		// configure conversion webhook
		if err := s.configureCRDConversionWebhook(clients); err != nil {
			return nil, fmt.Errorf("configure conversion webhook for CRD failed, error: %w", err)
		}

		return secret, nil
	})

	tlsName := fmt.Sprintf("%s.%s.svc", s.name, s.options.Namespace)

	return dls.ListenAndServe(s.context, s.options.HTTPSListenPort, 0, handler, &dls.ListenOpts{
		Secrets:       clients.Core.Secret(),
		CertNamespace: s.options.Namespace,
		CertName:      certName,
		CAName:        caName,
		TLSListenerConfig: dynamiclistener.Config{
			SANs: []string{
				tlsName,
			},
			FilterCN: dynamiclistener.OnlyAllow(tlsName),
			TLSConfig: &tls.Config{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: whiteListedCiphers,
			},
		},
	})
}

func (s *WebhookServer) validatingWebhookConfiguration() *v1.ValidatingWebhookConfiguration {
	if len(s.validators) == 0 {
		return nil
	}

	resources := make([]admission.Resource, 0, len(s.validators))
	for _, validator := range s.validators {
		resources = append(resources, validator.Resource())
	}

	return &v1.ValidatingWebhookConfiguration{
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
					CABundle: s.caBundle,
				},
				Rules:                   buildRules(resources),
				FailurePolicy:           &failPolicyFail,
				SideEffects:             &sideEffectClassNone,
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
			},
		},
	}
}

func (s *WebhookServer) mutatingWebhookConfiguration() *v1.MutatingWebhookConfiguration {
	if len(s.mutators) == 0 {
		return nil
	}

	resources := make([]admission.Resource, 0, len(s.mutators))
	for _, mutator := range s.mutators {
		resources = append(resources, mutator.Resource())
	}
	return &v1.MutatingWebhookConfiguration{
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
					CABundle: s.caBundle,
				},
				Rules:                   buildRules(resources),
				FailurePolicy:           &failPolicyIgnore,
				SideEffects:             &sideEffectClassNone,
				AdmissionReviewVersions: []string{"v1", "v1beta1"},
			},
		},
	}
}

func (s *WebhookServer) configureCRDConversionWebhook(clients *clients.Clients) error {
	crdClient := clients.CRD.CustomResourceDefinition()
	for _, converter := range s.converters {
		crd, err := crdClient.Get(converter.GroupResource().String(), metav1.GetOptions{})
		if err != nil {
			return err
		}
		// configure conversion webhook
		crdCopy := crd.DeepCopy()
		crdCopy.Spec.Conversion.Strategy = apiextensionsv1.WebhookConverter
		crdCopy.Spec.Conversion.Webhook = &apiextensionsv1.WebhookConversion{
			ConversionReviewVersions: []string{"v1"},
			ClientConfig: &apiextensionsv1.WebhookClientConfig{
				Service: &apiextensionsv1.ServiceReference{
					Namespace: s.options.Namespace,
					Name:      s.name,
					Path:      &conversionPath,
					Port:      &port,
				},
				CABundle: s.caBundle,
			},
		}
		if _, err := crdClient.Update(crdCopy); err != nil {
			return err
		}
	}

	return nil
}

// RegisterValidators registers validator to the webhook server.
// Call it before starting the webhook server.
func (s *WebhookServer) RegisterValidators(validators ...admission.Validator) error {
	if s.isStarted {
		return fmt.Errorf("cannot register validators after the webhook server is started")
	}

	s.validators = append(s.validators, validators...)
	return nil
}

// RegisterMutators registers mutator to the webhook server.
// Call it before start the webhook server.
func (s *WebhookServer) RegisterMutators(mutators ...admission.Mutator) error {
	if s.isStarted {
		return fmt.Errorf("cannot register mutators after the webhook server is started")
	}

	s.mutators = append(s.mutators, mutators...)
	return nil
}

// RegisterConverters registers converters to the webhook server.
// Call it before start the webhook server.
func (s *WebhookServer) RegisterConverters(converters ...conversion.Converter) error {
	if s.isStarted {
		return fmt.Errorf("cannot register converters after the webhook server is started")
	}

	s.converters = append(s.converters, converters...)
	return nil
}

func (s *WebhookServer) validatingHandler() http.Handler {
	if len(s.validators) == 0 {
		return nil
	}

	router := webhook.NewRouter()
	for _, v := range s.validators {
		h := admission.NewHandler(admission.Validator2Admitter(v), admission.AdmissionTypeValidation, s.options)
		h.AddToWebhookRouter(router)
	}

	return router
}

func (s *WebhookServer) mutatingHandler() http.Handler {
	if len(s.mutators) == 0 {
		return nil
	}

	router := webhook.NewRouter()
	for _, m := range s.mutators {
		h := admission.NewHandler(m, admission.AdmissionTypeMutation, s.options)
		h.AddToWebhookRouter(router)
	}

	return router
}

func buildRules(resources []admission.Resource) []v1.RuleWithOperations {
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
