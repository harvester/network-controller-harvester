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
	clientset "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned"
	kubeovnv1 "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/typed/kubeovn.io/v1"
	fakekubeovnv1 "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/typed/kubeovn.io/v1/fake"
	networkv1beta1 "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/typed/network.harvesterhci.io/v1beta1"
	fakenetworkv1beta1 "github.com/harvester/harvester-network-controller/pkg/generated/clientset/versioned/typed/network.harvesterhci.io/v1beta1/fake"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/testing"
)

// NewSimpleClientset returns a clientset that will respond with the provided objects.
// It's backed by a very simple object tracker that processes creates, updates and deletions as-is,
// without applying any field management, validations and/or defaults. It shouldn't be considered a replacement
// for a real clientset and is mostly useful in simple unit tests.
//
// DEPRECATED: NewClientset replaces this with support for field management, which significantly improves
// server side apply testing. NewClientset is only available when apply configurations are generated (e.g.
// via --with-applyconfig).
func NewSimpleClientset(objects ...runtime.Object) *Clientset {
	o := testing.NewObjectTracker(scheme, codecs.UniversalDecoder())
	for _, obj := range objects {
		if err := o.Add(obj); err != nil {
			panic(err)
		}
	}

	cs := &Clientset{tracker: o}
	cs.discovery = &fakediscovery.FakeDiscovery{Fake: &cs.Fake}
	cs.AddReactor("*", "*", testing.ObjectReaction(o))
	cs.AddWatchReactor("*", func(action testing.Action) (handled bool, ret watch.Interface, err error) {
		gvr := action.GetResource()
		ns := action.GetNamespace()
		watch, err := o.Watch(gvr, ns)
		if err != nil {
			return false, nil, err
		}
		return true, watch, nil
	})

	return cs
}

// Clientset implements clientset.Interface. Meant to be embedded into a
// struct to get a default implementation. This makes faking out just the method
// you want to test easier.
type Clientset struct {
	testing.Fake
	discovery *fakediscovery.FakeDiscovery
	tracker   testing.ObjectTracker
}

func (c *Clientset) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

func (c *Clientset) Tracker() testing.ObjectTracker {
	return c.tracker
}

var (
	_ clientset.Interface = &Clientset{}
	_ testing.FakeClient  = &Clientset{}
)

// KubeovnV1 retrieves the KubeovnV1Client
func (c *Clientset) KubeovnV1() kubeovnv1.KubeovnV1Interface {
	return &fakekubeovnv1.FakeKubeovnV1{Fake: &c.Fake}
}

// NetworkV1beta1 retrieves the NetworkV1beta1Client
func (c *Clientset) NetworkV1beta1() networkv1beta1.NetworkV1beta1Interface {
	return &fakenetworkv1beta1.FakeNetworkV1beta1{Fake: &c.Fake}
}
