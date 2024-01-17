package conversion

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Converter is a interface to convert object to the desired version
type Converter interface {
	GroupResource() schema.GroupResource
	Convert(Object *unstructured.Unstructured, desiredAPIVersion string) (*unstructured.Unstructured, error)
}

// Handler is a http handler for multiple converters
type Handler struct {
	restMapper meta.RESTMapper
	converters map[schema.GroupResource]Converter
}

func NewHandler(converters []Converter, restMapper meta.RESTMapper) *Handler {
	h := &Handler{
		restMapper: restMapper,
		converters: map[schema.GroupResource]Converter{},
	}
	for _, c := range converters {
		h.converters[c.GroupResource()] = c
	}

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	review := &apiextensionsv1.ConversionReview{}
	err := json.NewDecoder(r.Body).Decode(review)
	if err != nil {
		sendError(w, review, err)
		return
	}

	if review.Request == nil {
		sendError(w, review, fmt.Errorf("request is not set"))
		return
	}

	review.Response = h.doConversion(review.Request)
	review.Request = nil

	writeResponse(w, review)
}

func (h *Handler) doConversion(request *apiextensionsv1.ConversionRequest) *apiextensionsv1.ConversionResponse {
	response := &apiextensionsv1.ConversionResponse{
		UID: request.UID,
	}

	for _, obj := range request.Objects {
		convertedObj, err := h.convertObject(&obj, request.DesiredAPIVersion)
		if err != nil {
			response.Result = errors.NewInternalError(err).ErrStatus
			return response
		}
		response.ConvertedObjects = append(response.ConvertedObjects, *convertedObj)
	}

	response.Result = metav1.Status{
		Status: metav1.StatusSuccess,
	}

	return response
}

func (h *Handler) convertObject(obj *runtime.RawExtension, desiredAPIVersion string) (*runtime.RawExtension, error) {
	cr := &unstructured.Unstructured{}
	if err := cr.UnmarshalJSON(obj.Raw); err != nil {
		return nil, err
	}
	// use RESTMapping to get the group resource of the object
	mapper, err := h.restMapper.RESTMapping(cr.GroupVersionKind().GroupKind())
	if err != nil {
		return nil, err
	}
	groupResource := mapper.Resource.GroupResource()
	converter, ok := h.converters[groupResource]
	if !ok {
		return nil, fmt.Errorf("converter for %s is not existing", groupResource.String())
	}

	convertedObj, err := converter.Convert(cr, desiredAPIVersion)
	if err != nil {
		return nil, err
	}
	convertedObj.SetAPIVersion(desiredAPIVersion)

	return &runtime.RawExtension{Object: convertedObj}, nil
}

func writeResponse(w http.ResponseWriter, review *apiextensionsv1.ConversionReview) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(review); err != nil {
		logrus.Errorf("encode review failed, review: %+v, error: %v", review, err)
	}
}

func sendError(w http.ResponseWriter, review *apiextensionsv1.ConversionReview, err error) {
	logrus.Error(err)
	if review == nil || review.Request == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	review.Response.Result = errors.NewInternalError(err).ErrStatus
	writeResponse(w, review)
}
