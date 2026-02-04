module github.com/harvester/harvester-network-controller

go 1.25

toolchain go1.25.3

replace (
	github.com/containerd/containerd => github.com/containerd/containerd v1.6.18
	github.com/docker/distribution => github.com/docker/distribution v2.8.0+incompatible // oras dep requires a replace is set
	github.com/docker/docker => github.com/docker/docker v20.10.9+incompatible // oras dep requires a replace is set
	github.com/gin-gonic/gin => github.com/gin-gonic/gin v1.7.7
	github.com/knative/pkg => github.com/rancher/pkg v0.0.0-20190514055449-b30ab9de040e
	github.com/openshift/api => github.com/openshift/api v0.0.0-20191219222812-2987a591a72c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200521150516-05eb9880269c
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring => github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.72.0
	github.com/rancher/lasso => github.com/rancher/lasso v0.0.0-20240705194423-b2a060d103c1

	github.com/rancher/rancher => github.com/rancher/rancher v0.0.0-20240919204204-3da2ae0cabd1
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20240919204204-3da2ae0cabd1
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20240919204204-3da2ae0cabd1

	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp => go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.53.0
	go.opentelemetry.io/otel => go.opentelemetry.io/otel v1.33.0
	go.opentelemetry.io/otel/exporters/otlp => go.opentelemetry.io/otel/exporters/otlp v0.20.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace => go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.28.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc => go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.27.0
	go.opentelemetry.io/otel/metric => go.opentelemetry.io/otel/metric v1.33.0
	go.opentelemetry.io/otel/sdk => go.opentelemetry.io/otel/sdk v1.31.0
	go.opentelemetry.io/otel/trace => go.opentelemetry.io/otel/trace v1.33.0
	go.opentelemetry.io/proto/otlp => go.opentelemetry.io/proto/otlp v1.3.1
	golang.org/x/net => golang.org/x/net v0.38.0
	google.golang.org/grpc => google.golang.org/grpc v1.69.4
	helm.sh/helm/v3 => github.com/rancher/helm/v3 v3.9.0-rancher1
	k8s.io/api => k8s.io/api v0.33.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.33.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.33.5
	k8s.io/apiserver => k8s.io/apiserver v0.33.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.24.10
	k8s.io/client-go => k8s.io/client-go v0.33.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.24.10
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.10
	k8s.io/code-generator => k8s.io/code-generator v0.33.5
	k8s.io/component-base => k8s.io/component-base v0.33.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.24.10
	k8s.io/controller-manager => k8s.io/controller-manager v0.24.10
	k8s.io/cri-api => k8s.io/cri-api v0.24.10
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.24.10
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.24.10
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.24.10
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20250318190949-c8a335a9a2ff // indirect
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.24.10
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.24.10
	k8s.io/kubectl => k8s.io/kubectl v0.24.2
	k8s.io/kubelet => k8s.io/kubelet v0.24.10
	k8s.io/kubernetes => k8s.io/kubernetes v1.24.10
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.24.10
	k8s.io/metrics => k8s.io/metrics v0.24.10
	k8s.io/mount-utils => k8s.io/mount-utils v0.31.1
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.24.10
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.24.10
	kubevirt.io/api => github.com/kubevirt/api v1.7.0
	kubevirt.io/client-go => github.com/kubevirt/client-go v1.7.0
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 => github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.7.3
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.19.7
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)

require (
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2
	github.com/cenk/backoff v2.2.1+incompatible
	github.com/containernetworking/cni v1.3.0
	github.com/containernetworking/plugins v1.9.0
	github.com/coreos/go-iptables v0.8.0
	github.com/deckarep/golang-set/v2 v2.6.0
	github.com/go-ping/ping v0.0.0-20211014180314-6e2b003bffdd
	github.com/harvester/webhook v0.1.5
	github.com/insomniacslk/dhcp v0.0.0-20240829085014-a3a4c1f04475
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.7.5
	github.com/kubeovn/kube-ovn v1.13.13
	github.com/rancher/lasso v0.2.2
	github.com/rancher/wrangler v1.1.2
	github.com/rancher/wrangler/v3 v3.1.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.10.0
	github.com/tidwall/sjson v1.2.5
	github.com/urfave/cli v1.22.16
	github.com/vishvananda/netlink v1.3.1
	k8s.io/api v0.33.5
	k8s.io/apiextensions-apiserver v0.33.5
	k8s.io/apimachinery v0.33.5
	k8s.io/client-go v12.0.0+incompatible
	kubevirt.io/api v1.7.0
	sigs.k8s.io/controller-runtime v0.20.2
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/evanphx/json-patch v5.9.11+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-openapi/jsonpointer v0.21.1 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mdlayher/packet v1.1.2 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/ovn-org/libovsdb v0.7.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rancher/dynamiclistener v0.6.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/tidwall/gjson v1.14.2 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/u-root/uio v0.0.0-20240224005618-d2acac8f3701 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/mod v0.27.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
	golang.org/x/tools/go/packages/packagestest v0.1.1-deprecated // indirect
	google.golang.org/protobuf v1.36.7 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/code-generator v0.33.5 // indirect
	k8s.io/gengo v0.0.0-20250130153323-76c5745d3511 // indirect
	k8s.io/gengo/v2 v2.0.0-20250207200755-1244d31929d7 // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-aggregator v0.32.1 // indirect
	k8s.io/kube-openapi v0.31.9 // indirect
	k8s.io/utils v0.0.0-20250502105355-0f33e8f1c979 // indirect
	kubevirt.io/containerized-data-importer-api v1.63.1 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
