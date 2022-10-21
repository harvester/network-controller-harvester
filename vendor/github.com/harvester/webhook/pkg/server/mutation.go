package server

import (
	"net/http"
	"reflect"

	"github.com/rancher/wrangler/pkg/webhook"
	"github.com/sirupsen/logrus"

	"github.com/harvester/webhook/pkg/config"
	"github.com/harvester/webhook/pkg/types"
)

func (s *AdmissionWebhookServer) mutation(options *config.Options) (http.Handler, []types.Resource, error) {
	router := webhook.NewRouter()
	resources := make([]types.Resource, 0)
	for _, m := range s.admitters[types.AdmissionTypeMutation] {
		addHandler(router, types.AdmissionTypeMutation, m, options)
		resources = append(resources, m.Resource())
	}

	return router, resources, nil
}

func addHandler(router *webhook.Router, admissionType types.AdmissionType, admitter types.Admitter, options *config.Options) {
	rsc := admitter.Resource()
	kind := reflect.Indirect(reflect.ValueOf(rsc.ObjectType)).Type().Name()
	router.Kind(kind).Group(rsc.APIGroup).Type(rsc.ObjectType).Handle(types.NewAdmissionHandler(admitter, admissionType, options))
	logrus.Infof("add %s handler for %+v.%s (%s)", admissionType, rsc.Names, rsc.APIGroup, kind)
}
