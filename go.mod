module github.com/harvester/harvester-network-controller

go 1.24.0

toolchain go1.24.3

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
	k8s.io/api => k8s.io/api v0.31.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.31.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.31.5
	k8s.io/apiserver => k8s.io/apiserver v0.32.3
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.24.10
	k8s.io/client-go => k8s.io/client-go v0.31.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.24.10
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.10
	k8s.io/code-generator => k8s.io/code-generator v0.31.5
	k8s.io/component-base => k8s.io/component-base v0.31.5
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
	kubevirt.io/api => github.com/kubevirt/api v1.4.0
	kubevirt.io/client-go => github.com/kubevirt/client-go v1.4.0
	launchpad.net/gocheck v0.0.0-20140225173054-000000000087 => github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.7.3
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.19.7
	sigs.k8s.io/structured-merge-diff => sigs.k8s.io/structured-merge-diff v0.0.0-20190302045857-e85c7b244fd2
)

require (
	github.com/achanda/go-sysctl v0.0.0-20160222034550-6be7678c45d2
	github.com/cenk/backoff v2.2.1+incompatible
	github.com/containernetworking/cni v1.3.0
	github.com/coreos/go-iptables v0.6.0
	github.com/deckarep/golang-set/v2 v2.6.0
	github.com/go-ping/ping v0.0.0-20211014180314-6e2b003bffdd
	github.com/harvester/harvester v1.5.0
	github.com/harvester/webhook v0.1.5
	github.com/insomniacslk/dhcp v0.0.0-20240710054256-ddd8a41251c9
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.7.5
	github.com/kubeovn/kube-ovn v1.13.13
	github.com/rancher/lasso v0.2.2
	github.com/rancher/wrangler v1.1.2
	github.com/rancher/wrangler/v3 v3.1.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.10.0
	github.com/tidwall/sjson v1.2.5
	github.com/urfave/cli v1.22.16
	github.com/vishvananda/netlink v1.3.0
	k8s.io/api v0.33.0
	k8s.io/apimachinery v0.32.3
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.130.1
	kubevirt.io/api v1.4.0
)

require (
	emperror.dev/errors v0.8.1 // indirect
	github.com/adrg/xdg v0.4.0 // indirect
	github.com/aws/aws-sdk-go v1.55.6 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/c9s/goprocinfo v0.0.0-20210130143923-c95fcf8c64a8 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cisco-open/operator-tools v0.34.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/evanphx/json-patch v5.9.11+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/gammazero/deque v1.0.0 // indirect
	github.com/gammazero/workerpool v1.1.3 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-kit/kit v0.13.0 // indirect
	github.com/go-kit/log v0.2.1 // indirect
	github.com/go-logfmt/logfmt v0.6.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.1 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.1 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/glog v1.2.5 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/mock v1.6.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.9 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/handlers v1.5.2 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.26.3 // indirect
	github.com/harvester/go-common v0.0.0-20250109132713-e748ce72a7ba // indirect
	github.com/harvester/node-manager v0.3.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/iancoleman/orderedmap v0.3.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/k3s-io/helm-controller v0.16.1 // indirect
	github.com/k8snetworkplumbingwg/whereabouts v0.8.0 // indirect
	github.com/kube-logging/logging-operator/pkg/sdk v0.11.1-0.20240314152935-421fefebc813 // indirect
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0 // indirect
	github.com/kubernetes/dashboard v1.10.1 // indirect
	github.com/longhorn/backupstore v0.0.0-20250227220202-651bd33886fe // indirect
	github.com/longhorn/go-common-libs v0.0.0-20250215052214-151615b29f8e // indirect
	github.com/longhorn/longhorn-manager v1.8.1 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mdlayher/packet v1.1.2 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/ncruces/go-strftime v0.1.9 // indirect
	github.com/onsi/gomega v1.37.0 // indirect
	github.com/opencontainers/runc v1.1.13 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/openshift/api v0.0.0 // indirect
	github.com/openshift/client-go v3.9.0+incompatible // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/ovn-org/libovsdb v0.7.0 // indirect
	github.com/pborman/uuid v1.2.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.78.2 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rancher/aks-operator v1.9.2 // indirect
	github.com/rancher/apiserver v0.0.0-20240708202538-39a6f2535146 // indirect
	github.com/rancher/dynamiclistener v0.6.1 // indirect
	github.com/rancher/eks-operator v1.9.2 // indirect
	github.com/rancher/fleet/pkg/apis v0.10.0 // indirect
	github.com/rancher/gke-operator v1.9.2 // indirect
	github.com/rancher/kubernetes-provider-detector v0.1.5 // indirect
	github.com/rancher/norman v0.0.0-20240708202514-a0127673d1b9 // indirect
	github.com/rancher/rancher v0.0.0-20240618122559-b9ec494d4f6f // indirect
	github.com/rancher/rancher/pkg/apis v0.0.0 // indirect
	github.com/rancher/remotedialer v0.4.0 // indirect
	github.com/rancher/rke v1.6.2 // indirect
	github.com/rancher/steve v0.0.0-20240911190153-79304d93b49b // indirect
	github.com/rancher/system-upgrade-controller/pkg/apis v0.0.0-20240301001845-4eacc2dabbde // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/robfig/cron v1.2.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/slok/goresilience v0.2.0 // indirect
	github.com/spf13/cast v1.8.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/tidwall/gjson v1.14.2 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/u-root/uio v0.0.0-20230220225925-ffce2a382923 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.36.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.36.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk v1.36.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	go.opentelemetry.io/proto/otlp v1.6.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.36.0 // indirect
	golang.org/x/exp v0.0.0-20250506013437-ce4c2cf36ca6 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	golang.org/x/tools v0.33.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250519155744-55703ea1f237 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250519155744-55703ea1f237 // indirect
	google.golang.org/grpc v1.72.2 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.33.0 // indirect
	k8s.io/apiserver v0.33.0 // indirect
	k8s.io/code-generator v0.32.1 // indirect
	k8s.io/component-base v0.33.0 // indirect
	k8s.io/gengo v0.0.0-20250130153323-76c5745d3511 // indirect
	k8s.io/gengo/v2 v2.0.0-20240911193312-2b36238f13e9 // indirect
	k8s.io/kube-aggregator v0.32.1 // indirect
	k8s.io/kube-openapi v0.31.9 // indirect
	k8s.io/kubernetes v1.32.2 // indirect
	k8s.io/mount-utils v0.32.2 // indirect
	k8s.io/utils v0.0.0-20250502105355-0f33e8f1c979 // indirect
	kubevirt.io/client-go v1.4.0 // indirect
	kubevirt.io/containerized-data-importer-api v1.61.0 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	kubevirt.io/kubevirt v1.4.0 // indirect
	modernc.org/gc/v3 v3.0.0-20240107210532-573471604cb6 // indirect
	modernc.org/libc v1.49.3 // indirect
	modernc.org/mathutil v1.6.0 // indirect
	modernc.org/memory v1.8.0 // indirect
	modernc.org/sqlite v1.29.10 // indirect
	modernc.org/strutil v1.2.0 // indirect
	modernc.org/token v1.1.0 // indirect
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.32.1 // indirect
	sigs.k8s.io/cli-utils v0.37.2 // indirect
	sigs.k8s.io/cluster-api v1.7.3 // indirect
	sigs.k8s.io/controller-runtime v0.19.7 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
