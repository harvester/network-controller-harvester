package server

import (
	"net/http"

	"github.com/rancher/wrangler/pkg/webhook"

	"github.com/yaocw2020/webhook/pkg/config"
	"github.com/yaocw2020/webhook/pkg/types"
)

func (s *AdmissionWebhookServer) validation(options *config.Options) (http.Handler, []types.Resource, error) {
	router := webhook.NewRouter()
	resources := make([]types.Resource, 0)
	for _, v := range s.admitters[types.AdmissionTypeValidation] {
		addHandler(router, types.AdmissionTypeValidation, v, options)
		resources = append(resources, v.Resource())
	}

	return router, resources, nil
}
