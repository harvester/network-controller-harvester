package fakeclients

import (
	"context"

	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	kubeovnType "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/typed/kubeovn.io/v1"
)

type SubnetClient func() kubeovnType.SubnetInterface

type SubnetCache func() kubeovnType.SubnetInterface

func (v SubnetCache) Get(name string) (*kubeovnv1.Subnet, error) {
	return v().Get(context.TODO(), name, metav1.GetOptions{})
}

func (v SubnetCache) List(selector labels.Selector) ([]*kubeovnv1.Subnet, error) {
	list, err := v().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*kubeovnv1.Subnet, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (v SubnetCache) AddIndexer(_ string, _ generic.Indexer[*kubeovnv1.Subnet]) {
	panic("implement me")
}

func (v SubnetCache) GetByIndex(_, _ string) ([]*kubeovnv1.Subnet, error) {
	panic("implement me")
}
