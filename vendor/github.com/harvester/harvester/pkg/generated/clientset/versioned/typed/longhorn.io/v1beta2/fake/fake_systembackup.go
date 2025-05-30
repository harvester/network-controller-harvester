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

	v1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeSystemBackups implements SystemBackupInterface
type FakeSystemBackups struct {
	Fake *FakeLonghornV1beta2
	ns   string
}

var systembackupsResource = v1beta2.SchemeGroupVersion.WithResource("systembackups")

var systembackupsKind = v1beta2.SchemeGroupVersion.WithKind("SystemBackup")

// Get takes name of the systemBackup, and returns the corresponding systemBackup object, and an error if there is any.
func (c *FakeSystemBackups) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1beta2.SystemBackup, err error) {
	emptyResult := &v1beta2.SystemBackup{}
	obj, err := c.Fake.
		Invokes(testing.NewGetActionWithOptions(systembackupsResource, c.ns, name, options), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.SystemBackup), err
}

// List takes label and field selectors, and returns the list of SystemBackups that match those selectors.
func (c *FakeSystemBackups) List(ctx context.Context, opts v1.ListOptions) (result *v1beta2.SystemBackupList, err error) {
	emptyResult := &v1beta2.SystemBackupList{}
	obj, err := c.Fake.
		Invokes(testing.NewListActionWithOptions(systembackupsResource, systembackupsKind, c.ns, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1beta2.SystemBackupList{ListMeta: obj.(*v1beta2.SystemBackupList).ListMeta}
	for _, item := range obj.(*v1beta2.SystemBackupList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested systemBackups.
func (c *FakeSystemBackups) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchActionWithOptions(systembackupsResource, c.ns, opts))

}

// Create takes the representation of a systemBackup and creates it.  Returns the server's representation of the systemBackup, and an error, if there is any.
func (c *FakeSystemBackups) Create(ctx context.Context, systemBackup *v1beta2.SystemBackup, opts v1.CreateOptions) (result *v1beta2.SystemBackup, err error) {
	emptyResult := &v1beta2.SystemBackup{}
	obj, err := c.Fake.
		Invokes(testing.NewCreateActionWithOptions(systembackupsResource, c.ns, systemBackup, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.SystemBackup), err
}

// Update takes the representation of a systemBackup and updates it. Returns the server's representation of the systemBackup, and an error, if there is any.
func (c *FakeSystemBackups) Update(ctx context.Context, systemBackup *v1beta2.SystemBackup, opts v1.UpdateOptions) (result *v1beta2.SystemBackup, err error) {
	emptyResult := &v1beta2.SystemBackup{}
	obj, err := c.Fake.
		Invokes(testing.NewUpdateActionWithOptions(systembackupsResource, c.ns, systemBackup, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.SystemBackup), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeSystemBackups) UpdateStatus(ctx context.Context, systemBackup *v1beta2.SystemBackup, opts v1.UpdateOptions) (result *v1beta2.SystemBackup, err error) {
	emptyResult := &v1beta2.SystemBackup{}
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceActionWithOptions(systembackupsResource, "status", c.ns, systemBackup, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.SystemBackup), err
}

// Delete takes name of the systemBackup and deletes it. Returns an error if one occurs.
func (c *FakeSystemBackups) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(systembackupsResource, c.ns, name, opts), &v1beta2.SystemBackup{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeSystemBackups) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionActionWithOptions(systembackupsResource, c.ns, opts, listOpts)

	_, err := c.Fake.Invokes(action, &v1beta2.SystemBackupList{})
	return err
}

// Patch applies the patch and returns the patched systemBackup.
func (c *FakeSystemBackups) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1beta2.SystemBackup, err error) {
	emptyResult := &v1beta2.SystemBackup{}
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceActionWithOptions(systembackupsResource, c.ns, name, pt, data, opts, subresources...), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1beta2.SystemBackup), err
}
