/*
Copyright 2025 Harvester Network Controller Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by main. DO NOT EDIT.

package fake

import (
	"context"

	v1beta1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeVlanConfigs implements VlanConfigInterface
type FakeVlanConfigs struct {
	Fake *FakeNetworkV1beta1
}

var vlanconfigsResource = v1beta1.SchemeGroupVersion.WithResource("vlanconfigs")

var vlanconfigsKind = v1beta1.SchemeGroupVersion.WithKind("VlanConfig")

// Get takes name of the vlanConfig, and returns the corresponding vlanConfig object, and an error if there is any.
func (c *FakeVlanConfigs) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta1.VlanConfig, err error) {
	emptyResult := &v1beta1.VlanConfig{}
	obj, err := c.Fake.
		Invokes(testing.NewRootGetActionWithOptions(vlanconfigsResource, name, options), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta1.VlanConfig), err
}

// List takes label and field selectors, and returns the list of VlanConfigs that match those selectors.
func (c *FakeVlanConfigs) List(ctx context.Context, opts v1.ListOptions) (result *v1beta1.VlanConfigList, err error) {
	emptyResult := &v1beta1.VlanConfigList{}
	obj, err := c.Fake.
		Invokes(testing.NewRootListActionWithOptions(vlanconfigsResource, vlanconfigsKind, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta1.VlanConfigList{ListMeta: obj.(*v1beta1.VlanConfigList).ListMeta}
	for _, item := range obj.(*v1beta1.VlanConfigList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested vlanConfigs.
func (c *FakeVlanConfigs) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchActionWithOptions(vlanconfigsResource, opts))
}

// Create takes the representation of a vlanConfig and creates it.  Returns the server's representation of the vlanConfig, and an error, if there is any.
func (c *FakeVlanConfigs) Create(ctx context.Context, vlanConfig *v1beta1.VlanConfig, opts v1.CreateOptions) (result *v1beta1.VlanConfig, err error) {
	emptyResult := &v1beta1.VlanConfig{}
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateActionWithOptions(vlanconfigsResource, vlanConfig, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta1.VlanConfig), err
}

// Update takes the representation of a vlanConfig and updates it. Returns the server's representation of the vlanConfig, and an error, if there is any.
func (c *FakeVlanConfigs) Update(ctx context.Context, vlanConfig *v1beta1.VlanConfig, opts v1.UpdateOptions) (result *v1beta1.VlanConfig, err error) {
	emptyResult := &v1beta1.VlanConfig{}
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateActionWithOptions(vlanconfigsResource, vlanConfig, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta1.VlanConfig), err
}

// Delete takes name of the vlanConfig and deletes it. Returns an error if one occurs.
func (c *FakeVlanConfigs) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(vlanconfigsResource, name, opts), &v1beta1.VlanConfig{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeVlanConfigs) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionActionWithOptions(vlanconfigsResource, opts, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta1.VlanConfigList{})
	return err
}

// Patch applies the patch and returns the patched vlanConfig.
func (c *FakeVlanConfigs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta1.VlanConfig, err error) {
	emptyResult := &v1beta1.VlanConfig{}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceActionWithOptions(vlanconfigsResource, name, pt, data, opts, subresources...), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta1.VlanConfig), err
}
