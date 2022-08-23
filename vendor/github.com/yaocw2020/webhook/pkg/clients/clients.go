package clients

import (
	"github.com/rancher/wrangler/pkg/clients"
	"github.com/rancher/wrangler/pkg/schemes"
	v1 "k8s.io/api/admissionregistration/v1"
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

	if err := schemes.Register(v1.AddToScheme); err != nil {
		return nil, err
	}

	return &Clients{
		Clients: *c,
	}, nil
}
