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

package v1

import (
	"context"

	scheme "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/scheme"
	v1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// VipsGetter has a method to return a VipInterface.
// A group's client should implement this interface.
type VipsGetter interface {
	Vips() VipInterface
}

// VipInterface has methods to work with Vip resources.
type VipInterface interface {
	Create(ctx context.Context, vip *v1.Vip, opts metav1.CreateOptions) (*v1.Vip, error)
	Update(ctx context.Context, vip *v1.Vip, opts metav1.UpdateOptions) (*v1.Vip, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, vip *v1.Vip, opts metav1.UpdateOptions) (*v1.Vip, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.Vip, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.VipList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.Vip, err error)
	VipExpansion
}

// vips implements VipInterface
type vips struct {
	*gentype.ClientWithList[*v1.Vip, *v1.VipList]
}

// newVips returns a Vips
func newVips(c *KubeovnV1Client) *vips {
	return &vips{
		gentype.NewClientWithList[*v1.Vip, *v1.VipList](
			"vips",
			c.RESTClient(),
			scheme.ParameterCodec,
			"",
			func() *v1.Vip { return &v1.Vip{} },
			func() *v1.VipList { return &v1.VipList{} }),
	}
}
