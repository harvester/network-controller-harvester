package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	nodetype "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/typed/v1"
)

type NodeClient func() nodetype.NodeInterface

func (c NodeClient) Create(s *v1.Node) (*v1.Node, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c NodeClient) Update(s *v1.Node) (*v1.Node, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c NodeClient) UpdateStatus(_ *v1.Node) (*v1.Node, error) {
	panic("implement me")
}

func (c NodeClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c NodeClient) Get(name string, options metav1.GetOptions) (*v1.Node, error) {
	return c().Get(context.TODO(), name, options)
}

func (c NodeClient) List(opts metav1.ListOptions) (*v1.NodeList, error) {
	return c().List(context.TODO(), opts)
}

func (c NodeClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c NodeClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.Node, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type NodeCache func() nodetype.NodeInterface

func (c NodeCache) Get(name string) (*v1.Node, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c NodeCache) List(selector labels.Selector) ([]*v1.Node, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*v1.Node, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c NodeCache) AddIndexer(_ string, _ generic.Indexer[*v1.Node]) {
	panic("implement me")
}

func (c NodeCache) GetByIndex(_, _ string) ([]*v1.Node, error) {
	panic("implement me")
}
