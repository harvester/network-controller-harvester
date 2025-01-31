module github.com/harvester/harvester-network-controller

go 1.22.8

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.6.18
	github.com/docker/distribution => github.com/docker/distribution v2.8.0+incompatible // oras dep requires a replace is set
	github.com/docker/docker => github.com/docker/docker v20.10.9+incompatible // oras dep requires a replace is set
	github.com/gin-gonic/gin => github.com/gin-gonic/gin v1.7.7
	github.com/knative/pkg => github.com/rancher/pkg v0.0.0-20190514055449-b30ab9de040e
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a

	github.com/rancher/rancher => github.com/rancher/rancher v0.0.0-20230124173128-2207cfed1803
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20230124173128-2207cfed1803
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20230124173128-2207cfed1803

	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp => go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.20.0
	go.opentelemetry.io/otel => go.opentelemetry.io/otel v0.20.0
	go.opentelemetry.io/otel/exporters/otlp => go.opentelemetry.io/otel/exporters/otlp v0.20.0
	go.opentelemetry.io/otel/sdk => go.opentelemetry.io/otel/sdk v0.20.0
	go.opentelemetry.io/otel/trace => go.opentelemetry.io/otel/trace v0.20.0
	go.opentelemetry.io/proto/otlp => go.opentelemetry.io/proto/otlp v0.7.0
	golang.org/x/net => golang.org/x/net v0.33.0
	google.golang.org/grpc => google.golang.org/grpc v1.56.3
	helm.sh/helm/v3 => github.com/rancher/helm/v3 v3.9.0-rancher1
	k8s.io/api => k8s.io/api v0.24.10
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.24.10
	k8s.io/apimachinery => k8s.io/apimachinery v0.24.10
	k8s.io/apiserver => k8s.io/apiserver v0.24.10
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.24.10
	k8s.io/client-go => k8s.io/client-go v0.24.10
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.24.10
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.10
	k8s.io/code-generator => k8s.io/code-generator v0.24.10
	k8s.io/component-base => k8s.io/component-base v0.24.10
	k8s.io/component-helpers => k8s.io/component-helpers v0.24.10
	k8s.io/controller-manager => k8s.io/controller-manager v0.24.10
	k8s.io/cri-api => k8s.io/cri-api v0.24.10
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.24.10
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.24.10
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.24.10
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.24.10
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.24.10
	k8s.io/kubectl => k8s.io/kubectl v0.24.2
	k8s.io/kubelet => k8s.io/kubelet v0.24.10
	k8s.io/kubernetes => k8s.io/kubernetes v1.24.10
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.24.10
	k8s.io/metrics => k8s.io/metrics v0.24.10
	k8s.io/mount-utils => k8s.io/mount-utils v0.24.10
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.24.10
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.24.10
	kubevirt.io/api => github.com/kubevirt/api v0.54.0
	kubevirt.io/client-go => github.com/kubevirt/client-go v0.54.0
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 => github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.1.4
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)

require (
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2
	github.com/cenk/backoff v2.2.1+incompatible
	github.com/containernetworking/cni v1.1.2
	github.com/coreos/go-iptables v0.6.0
	github.com/deckarep/golang-set/v2 v2.1.0
	github.com/go-ping/ping v0.0.0-20211014180314-6e2b003bffdd
	github.com/harvester/harvester v1.1.2-rc8
	github.com/harvester/webhook v0.1.4
	github.com/insomniacslk/dhcp v0.0.0-20240710054256-ddd8a41251c9
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200331171230-d50e42f2b669
	github.com/rancher/lasso v0.0.0-20221227210133-6ea88ca2fbcc
	github.com/rancher/wrangler v1.1.1
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.8.3
	github.com/tidwall/sjson v1.2.5
	github.com/urfave/cli v1.22.9
	github.com/vishvananda/netlink v1.2.1-beta.2
	k8s.io/api v0.25.4
	k8s.io/apimachinery v0.25.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.80.1
	kubevirt.io/api v0.54.0
)

require (
	emperror.dev/errors v0.8.0 // indirect
	github.com/PuerkitoBio/purell v1.1.1 // indirect
	github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578 // indirect
	github.com/adrg/xdg v0.3.1 // indirect
	github.com/banzaicloud/logging-operator/pkg/sdk v0.8.16 // indirect
	github.com/banzaicloud/operator-tools v0.28.10 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.8.0 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/fsnotify/fsnotify v1.5.4 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.6 // indirect
	github.com/go-openapi/swag v0.21.1 // indirect
	github.com/gobuffalo/flect v0.2.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/handlers v1.5.1 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/iancoleman/orderedmap v0.2.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/jinzhu/copier v0.3.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k3s-io/helm-controller v0.11.7 // indirect
	github.com/kubernetes-csi/external-snapshotter/v2 v2.1.3 // indirect
	github.com/kubernetes/dashboard v1.10.1 // indirect
	github.com/longhorn/go-iscsi-helper v0.0.0-20220805034259-7b59e22574bb // indirect
	github.com/longhorn/longhorn-manager v1.3.1 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mdlayher/packet v1.1.2 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/onsi/gomega v1.20.1 // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.62.0 // indirect
	github.com/prometheus/client_golang v1.12.2 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rancher/aks-operator v1.0.7 // indirect
	github.com/rancher/apiserver v0.0.0-20230120214941-e88c32739dc7 // indirect
	github.com/rancher/dynamiclistener v0.3.5 // indirect
	github.com/rancher/eks-operator v1.1.5 // indirect
	github.com/rancher/fleet/pkg/apis v0.0.0-20230123175930-d296259590be // indirect
	github.com/rancher/gke-operator v1.1.4 // indirect
	github.com/rancher/kubernetes-provider-detector v0.1.5 // indirect
	github.com/rancher/norman v0.0.0-20221205184727-32ef2e185b99 // indirect
	github.com/rancher/rancher v0.0.0-20230124173128-2207cfed1803 // indirect
	github.com/rancher/rancher/pkg/apis v0.0.0 // indirect
	github.com/rancher/remotedialer v0.2.6-0.20220624190122-ea57207bf2b8 // indirect
	github.com/rancher/rke v1.3.18 // indirect
	github.com/rancher/steve v0.0.0-20221209194631-acf9d31ce0dd // indirect
	github.com/rancher/system-upgrade-controller/pkg/apis v0.0.0-20210727200656-10b094e30007 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/tidwall/gjson v1.14.2 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/u-root/uio v0.0.0-20230220225925-ffce2a382923 // indirect
	github.com/vishvananda/netns v0.0.0-20211101163701-50045581ed74 // indirect
	go.opentelemetry.io/contrib v0.20.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.20.0 // indirect
	go.opentelemetry.io/otel v1.6.3 // indirect
	go.opentelemetry.io/otel/exporters/otlp v0.20.0 // indirect
	go.opentelemetry.io/otel/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk v1.3.0 // indirect
	go.opentelemetry.io/otel/sdk/export/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/trace v1.6.3 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.uber.org/atomic v1.8.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/oauth2 v0.7.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/term v0.27.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/grpc v1.54.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.25.4 // indirect
	k8s.io/apiserver v0.25.4 // indirect
	k8s.io/code-generator v0.25.4 // indirect
	k8s.io/component-base v0.25.4 // indirect
	k8s.io/gengo v0.0.0-20211129171323-c02415ce4185 // indirect
	k8s.io/kube-aggregator v0.25.4 // indirect
	k8s.io/kube-openapi v0.0.0-20220803162953-67bda5d908f1 // indirect
	k8s.io/utils v0.0.0-20221108210102-8e77b1f39fe2 // indirect
	kubevirt.io/containerized-data-importer-api v1.47.0 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	kubevirt.io/kubevirt v0.54.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.35 // indirect
	sigs.k8s.io/cli-utils v0.27.0 // indirect
	sigs.k8s.io/cluster-api v1.2.0-beta.0 // indirect
	sigs.k8s.io/controller-runtime v0.13.1 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)
