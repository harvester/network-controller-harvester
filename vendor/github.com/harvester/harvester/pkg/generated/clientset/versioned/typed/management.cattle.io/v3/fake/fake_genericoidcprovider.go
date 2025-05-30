/*
Copyright 2025 Rancher Labs, Inc.

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

	v3 "github.com/rancher/rancher/pkg/apis/management.cattle.io/v3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeGenericOIDCProviders implements GenericOIDCProviderInterface
type FakeGenericOIDCProviders struct {
	Fake *FakeManagementV3
}

var genericoidcprovidersResource = v3.SchemeGroupVersion.WithResource("genericoidcproviders")

var genericoidcprovidersKind = v3.SchemeGroupVersion.WithKind("GenericOIDCProvider")

// Get takes name of the genericOIDCProvider, and returns the corresponding genericOIDCProvider object, and an error if there is any.
func (c *FakeGenericOIDCProviders) Get(ctx context.Context, name string, options v1.GetOptions) (result *v3.GenericOIDCProvider, err error) {
	emptyResult := &v3.GenericOIDCProvider{}
	obj, err := c.Fake.
		Invokes(testing.NewRootGetActionWithOptions(genericoidcprovidersResource, name, options), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.GenericOIDCProvider), err
}

// List takes label and field selectors, and returns the list of GenericOIDCProviders that match those selectors.
func (c *FakeGenericOIDCProviders) List(ctx context.Context, opts v1.ListOptions) (result *v3.GenericOIDCProviderList, err error) {
	emptyResult := &v3.GenericOIDCProviderList{}
	obj, err := c.Fake.
		Invokes(testing.NewRootListActionWithOptions(genericoidcprovidersResource, genericoidcprovidersKind, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v3.GenericOIDCProviderList{ListMeta: obj.(*v3.GenericOIDCProviderList).ListMeta}
	for _, item := range obj.(*v3.GenericOIDCProviderList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested genericOIDCProviders.
func (c *FakeGenericOIDCProviders) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchActionWithOptions(genericoidcprovidersResource, opts))
}

// Create takes the representation of a genericOIDCProvider and creates it.  Returns the server's representation of the genericOIDCProvider, and an error, if there is any.
func (c *FakeGenericOIDCProviders) Create(ctx context.Context, genericOIDCProvider *v3.GenericOIDCProvider, opts v1.CreateOptions) (result *v3.GenericOIDCProvider, err error) {
	emptyResult := &v3.GenericOIDCProvider{}
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateActionWithOptions(genericoidcprovidersResource, genericOIDCProvider, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.GenericOIDCProvider), err
}

// Update takes the representation of a genericOIDCProvider and updates it. Returns the server's representation of the genericOIDCProvider, and an error, if there is any.
func (c *FakeGenericOIDCProviders) Update(ctx context.Context, genericOIDCProvider *v3.GenericOIDCProvider, opts v1.UpdateOptions) (result *v3.GenericOIDCProvider, err error) {
	emptyResult := &v3.GenericOIDCProvider{}
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateActionWithOptions(genericoidcprovidersResource, genericOIDCProvider, opts), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.GenericOIDCProvider), err
}

// Delete takes name of the genericOIDCProvider and deletes it. Returns an error if one occurs.
func (c *FakeGenericOIDCProviders) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteActionWithOptions(genericoidcprovidersResource, name, opts), &v3.GenericOIDCProvider{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeGenericOIDCProviders) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionActionWithOptions(genericoidcprovidersResource, opts, listOpts)

	_, err := c.Fake.Invokes(action, &v3.GenericOIDCProviderList{})
	return err
}

// Patch applies the patch and returns the patched genericOIDCProvider.
func (c *FakeGenericOIDCProviders) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v3.GenericOIDCProvider, err error) {
	emptyResult := &v3.GenericOIDCProvider{}
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceActionWithOptions(genericoidcprovidersResource, name, pt, data, opts, subresources...), emptyResult)
	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v3.GenericOIDCProvider), err
}
