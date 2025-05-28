package fakeclients

import (
	"context"

	kubevirtv1 "github.com/harvester/harvester/pkg/generated/clientset/versioned/typed/kubevirt.io/v1"
	"github.com/rancher/wrangler/v3/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	kubevirtv1api "kubevirt.io/api/core/v1"

	"github.com/harvester/harvester-network-controller/pkg/utils"
)

type VirtualMachineClient func(string) kubevirtv1.VirtualMachineInterface

func (c VirtualMachineClient) Update(vmi *kubevirtv1api.VirtualMachine) (*kubevirtv1api.VirtualMachine, error) {
	return c(vmi.Namespace).Update(context.TODO(), vmi, metav1.UpdateOptions{})
}

func (c VirtualMachineClient) Get(namespace, name string, options metav1.GetOptions) (*kubevirtv1api.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, options)
}

func (c VirtualMachineClient) Create(vmi *kubevirtv1api.VirtualMachine) (*kubevirtv1api.VirtualMachine, error) {
	return c(vmi.Namespace).Create(context.TODO(), vmi, metav1.CreateOptions{})
}

func (c VirtualMachineClient) Delete(_, _ string, _ *metav1.DeleteOptions) error {
	panic("implement me")
}

func (c VirtualMachineClient) List(_ string, _ metav1.ListOptions) (*kubevirtv1api.VirtualMachineList, error) {
	panic("implement me")
}

func (c VirtualMachineClient) UpdateStatus(*kubevirtv1api.VirtualMachine) (*kubevirtv1api.VirtualMachine, error) {
	panic("implement me")
}

func (c VirtualMachineClient) Watch(_ string, _ metav1.ListOptions) (watch.Interface, error) {
	panic("implement me")
}

func (c VirtualMachineClient) Patch(_, _ string, _ types.PatchType, _ []byte, _ ...string) (result *kubevirtv1api.VirtualMachine, err error) {
	panic("implement me")
}

type VirtualMachineCache func(string) kubevirtv1.VirtualMachineInterface

func (c VirtualMachineCache) Get(namespace, name string) (*kubevirtv1api.VirtualMachine, error) {
	return c(namespace).Get(context.TODO(), name, metav1.GetOptions{})
}

func (c VirtualMachineCache) List(namespace string, selector labels.Selector) ([]*kubevirtv1api.VirtualMachine, error) {
	list, err := c(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
	if err != nil {
		return nil, err
	}
	result := make([]*kubevirtv1api.VirtualMachine, 0, len(list.Items))
	for i := range list.Items {
		result = append(result, &list.Items[i])
	}
	return result, err
}

// support VMByNetworkIndex for test
func (c VirtualMachineCache) AddIndexer(index string, _ generic.Indexer[*kubevirtv1api.VirtualMachine]) {
	if index != utils.VMByNetworkIndex {
		panic("implement me")
	}
}

// support VMByNetworkIndex for test
func (c VirtualMachineCache) GetByIndex(index, key string) ([]*kubevirtv1api.VirtualMachine, error) {
	if index != utils.VMByNetworkIndex {
		panic("implement me")
	}

	// get from all namespace
	vms, err := c.List(corev1.NamespaceAll, labels.Everything())
	if err != nil {
		return nil, err
	}
	if len(vms) == 0 {
		return nil, nil
	}

	result := make([]*kubevirtv1api.VirtualMachine, 0)
	for _, vm := range vms {
		networks := vm.Spec.Template.Spec.Networks
		for _, network := range networks {
			if network.NetworkSource.Multus == nil {
				continue
			}
			if network.NetworkSource.Multus.NetworkName == key {
				result = append(result, vm)
			}
		}
	}

	return result, nil
}
