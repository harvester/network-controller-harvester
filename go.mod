module github.com/rancher/harvester-network-controller

go 1.13

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v12.4.3+incompatible
	github.com/crewjam/saml => github.com/rancher/saml v0.0.0-20180713225824-ce1532152fde
	github.com/dgrijalva/jwt-go => github.com/dgrijalva/jwt-go v3.2.1-0.20200107013213-dc14462fd587+incompatible
	github.com/rancher/apiserver => github.com/cnrancher/apiserver v0.0.0-20200731031228-a0459feeb0de
	github.com/rancher/steve => github.com/cnrancher/steve v0.0.0-20200922090254-a3cedc4d23cd
	k8s.io/client-go => k8s.io/client-go v0.18.0
	k8s.io/code-generator => k8s.io/code-generator v0.18.3
	kubevirt.io/client-go => github.com/cnrancher/kubevirt-client-go v0.31.1-0.20200715061104-844cb60487e4
	kubevirt.io/containerized-data-importer => github.com/cnrancher/kubevirt-containerized-data-importer v1.22.0-apis-only
)

require (
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v0.0.0-20200331171230-d50e42f2b669
	github.com/rancher/harvester v0.0.2-0.20201111020716-69a1fe84e370
	github.com/rancher/wrangler v0.6.2-0.20200622171942-7224e49a2407
	github.com/sirupsen/logrus v1.5.0
	github.com/urfave/cli v1.22.2
	github.com/vishvananda/netlink v1.1.0
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
)
