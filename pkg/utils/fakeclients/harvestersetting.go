package fakeclients

import (
	"context"

	"github.com/rancher/wrangler/v3/pkg/generic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"

	harvestersetting "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/typed/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

type SettingClient func() harvestersetting.SettingInterface

func (c SettingClient) Create(s *v1beta1.Setting) (*v1beta1.Setting, error) {
	return c().Create(context.TODO(), s, metav1.CreateOptions{})
}

func (c SettingClient) Update(s *v1beta1.Setting) (*v1beta1.Setting, error) {
	return c().Update(context.TODO(), s, metav1.UpdateOptions{})
}

func (c SettingClient) UpdateStatus(_ *v1beta1.Setting) (*v1beta1.Setting, error) {
	panic("implement me")
}

func (c SettingClient) Delete(name string, options *metav1.DeleteOptions) error {
	return c().Delete(context.TODO(), name, *options)
}

func (c SettingClient) Get(name string, options metav1.GetOptions) (*v1beta1.Setting, error) {
	return c().Get(context.TODO(), name, options)
}

func (c SettingClient) List(opts metav1.ListOptions) (*v1beta1.SettingList, error) {
	return c().List(context.TODO(), opts)
}

func (c SettingClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return c().Watch(context.TODO(), opts)
}

func (c SettingClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1beta1.Setting, err error) {
	return c().Patch(context.TODO(), name, pt, data, metav1.PatchOptions{}, subresources...)
}

type SettingCache func() harvestersetting.SettingInterface

func (c SettingCache) Get(name string) (*v1beta1.Setting, error) {
	return c().Get(context.TODO(), name, metav1.GetOptions{})
}

func (c SettingCache) List(selector labels.Selector) ([]*v1beta1.Setting, error) {
	list, err := c().List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, err
	}
	result := make([]*v1beta1.Setting, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

func (c SettingCache) AddIndexer(_ string, _ generic.Indexer[*v1beta1.Setting]) {
	panic("implement me")
}

func (c SettingCache) GetByIndex(_, _ string) ([]*v1beta1.Setting, error) {
	panic("implement me")
}
