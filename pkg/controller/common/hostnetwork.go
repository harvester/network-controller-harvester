package common

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	networkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/apis/network.harvester.cattle.io/v1alpha1"
	ctlnetworkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
)

const (
	HostNetworkNamespace = "harvester-system"
)

func NewHostNetworkFromNode(node *corev1.Node, networkSetting *NetworkSetting) *networkv1alpha1.HostNetwork {
	var networkSettingType = networkv1alpha1.NetworkTypeVLAN
	if networkSetting.Type != "" {
		networkSettingType = networkv1alpha1.NetworkType(networkSetting.Type)
	}
	return &networkv1alpha1.HostNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      node.Name,
			Namespace: HostNetworkNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Node",
					Name:       node.Name,
					UID:        node.UID,
				},
			},
		},
		Spec: networkv1alpha1.HostNetworkSpec{
			Type: networkSettingType,
			NIC:  networkSetting.NIC,
		},
		Status: networkv1alpha1.HostNetworkStatus{
			NetworkIDs:        []networkv1alpha1.NetworkID{},
			NetworkLinkStatus: map[string]*networkv1alpha1.LinkStatus{},
			Conditions:        []networkv1alpha1.Condition{},
		},
	}
}

func CreateHostNetworkIfNotExist(node *corev1.Node, networkSetting *NetworkSetting,
	hostNetworkCache ctlnetworkv1alpha1.HostNetworkCache,
	hostNetworkClient ctlnetworkv1alpha1.HostNetworkClient) (*networkv1alpha1.HostNetwork, error) {
	_, err := hostNetworkCache.Get(HostNetworkNamespace, node.Name)
	if err == nil {
		return nil, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	hostNetwork, err := hostNetworkClient.Create(NewHostNetworkFromNode(node, networkSetting))
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, nil
		}
		return nil, err
	}

	return hostNetwork, nil
}

func DeleteAllHostNetwork(hostNetworkCache ctlnetworkv1alpha1.HostNetworkCache,
	hostNetworkClient ctlnetworkv1alpha1.HostNetworkClient) error {
	hostNetworkList, err := hostNetworkCache.List(HostNetworkNamespace, labels.Everything())
	if err != nil {
		return err
	}
	for _, hostNetwork := range hostNetworkList {
		if err = hostNetworkClient.Delete(HostNetworkNamespace, hostNetwork.Name, &metav1.DeleteOptions{}); err != nil {
			return err
		}
	}

	return nil
}
