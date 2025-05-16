package clients

import (
	"github.com/rancher/wrangler/v3/pkg/clients"
	"github.com/rancher/wrangler/v3/pkg/schemes"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"
)

type Clients struct {
	clients.Clients
}

func New(rest *rest.Config) (*Clients, error) {
	c, err := clients.NewFromConfig(rest, nil)
	if err != nil {
		return nil, err
	}

	if err := schemes.Register(admissionv1.AddToScheme); err != nil {
		return nil, err
	}
	if err := schemes.Register(apiextv1.AddToScheme); err != nil {
		return nil, err
	}

	return &Clients{
		Clients: *c,
	}, nil
}
