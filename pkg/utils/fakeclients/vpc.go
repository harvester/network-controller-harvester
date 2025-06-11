package fakeclients

import (
	"context"

	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	kubeovnType "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/typed/kubeovn.io/v1"
)

type VpcClient func() kubeovnType.VpcInterface

type VpcCache func() kubeovnType.VpcInterface

func (v VpcCache) Get(name string) (*kubeovnv1.Vpc, error) {
	return v().Get(context.TODO(), name, metav1.GetOptions{})
}

func (v VpcCache) List(selector labels.Selector) ([]*kubeovnv1.Vpc, error) {
	list, err := v().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*kubeovnv1.Vpc, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (v VpcCache) AddIndexer(_ string, _ generic.Indexer[*kubeovnv1.Vpc]) {
	panic("implement me")
}

func (v VpcCache) GetByIndex(_, _ string) ([]*kubeovnv1.Vpc, error) {
	panic("implement me")
}
