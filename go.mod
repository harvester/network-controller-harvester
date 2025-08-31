module github.com/rancher/harvester-network-controller

go 1.13

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.2.0+incompatible
	github.com/crewjam/saml => github.com/rancher/saml v0.0.0-20180713225824-ce1532152fde
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go 9742bd7fca1c
	github.com/rancher/apiserver => github.com/cnrancher/apiserver 4388bb184a8e
	github.com/rancher/steve => github.com/cnrancher/steve f7a87efdab1b
	k8s.io/client-go => k8s.io/client-go v0.18.0
	k8s.io/code-generator => k8s.io/code-generator v0.18.3
	kubevirt.io/client-go => github.com/cnrancher/kubevirt-client-go 4cc1d98fcf55
	kubevirt.io/containerized-data-importer => github.com/cnrancher/kubevirt-containerized-data-importer v1.22.0-apis-only
)

require (
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200331171230-d50e42f2b669
	github.com/rancher/harvester v0.0.2-0.20201111020716-69a1fe84e370
	github.com/rancher/wrangler v0.6.2-0.20200622171942-7224e49a2407
	github.com/sirupsen/logrus v1.5.0
	github.com/urfave/cli v1.22.17
	github.com/vishvananda/netlink v1.1.0
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
)
