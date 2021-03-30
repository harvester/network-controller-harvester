package common

import (
	"fmt"

	v1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	networkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/apis/network.harvester.cattle.io/v1alpha1"
	ctlnetworkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
)

const (
	Namespace        = "harvester-system"
	KeyNodeName      = "NODENAME"
	KeyLabelNodeName = "network.harvester.cattle.io/nodename"
	initStatusMsg    = "Initializing network configuration"
)

func NewNodeNetworkFromNode(node *corev1.Node, networkType networkv1alpha1.NetworkType,
	cn *networkv1alpha1.ClusterNetwork) *networkv1alpha1.NodeNetwork {
	nn := &networkv1alpha1.NodeNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      node.Name + "-" + string(networkType),
			Namespace: Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Node",
					Name:       node.Name,
					UID:        node.UID,
				},
			},
			Labels: map[string]string{
				KeyLabelNodeName: node.Name,
			},
		},
		Spec: networkv1alpha1.NodeNetworkSpec{
			Type:     networkType,
			NodeName: node.Name,
		},
		Status: networkv1alpha1.NodeNetworkStatus{
			NetworkIDs:        []networkv1alpha1.NetworkID{},
			NetworkLinkStatus: map[string]*networkv1alpha1.LinkStatus{},
			Conditions:        []networkv1alpha1.Condition{},
		},
	}

	// initialize status
	networkv1alpha1.NodeNetworkReady.SetStatusBool(nn, false)
	networkv1alpha1.NodeNetworkReady.Message(nn, initStatusMsg)

	switch networkType {
	case networkv1alpha1.NetworkTypeVLAN:
		defaultPhysicalNIC, ok := cn.Config[networkv1alpha1.KeyDefaultNIC]
		if ok {
			nn.Spec.NIC = defaultPhysicalNIC
		}
	default:
	}

	return nn
}

func CreateNodeNetworkIfNotExist(node *corev1.Node, networkTye networkv1alpha1.NetworkType,
	cn *networkv1alpha1.ClusterNetwork, nodeNetworkCache ctlnetworkv1alpha1.NodeNetworkCache,
	nodeNetworkClient ctlnetworkv1alpha1.NodeNetworkClient) (*networkv1alpha1.NodeNetwork, error) {
	crName := node.Name + "-" + string(networkTye)
	nn, err := nodeNetworkCache.Get(Namespace, crName)
	if err == nil {
		return nn, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("get nn %s failed, error: %w", crName, err)
	}

	nodeNetwork, err := nodeNetworkClient.Create(NewNodeNetworkFromNode(node, networkTye, cn))
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, nil
		}
		return nil, err
	}

	return nodeNetwork, nil
}

func DeleteAllNodeNetwork(networkTye networkv1alpha1.NetworkType, nodeNetworkCache ctlnetworkv1alpha1.NodeNetworkCache,
	nodeNetworkClient ctlnetworkv1alpha1.NodeNetworkClient) error {
	nodeNetworkList, err := nodeNetworkCache.List(Namespace, labels.Everything())
	if err != nil {
		return err
	}
	for _, nodeNetwork := range nodeNetworkList {
		if nodeNetwork.Spec.Type == networkTye {
			if err = nodeNetworkClient.Delete(Namespace, nodeNetwork.Name, &metav1.DeleteOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}

func CreateAllNodeNetworkIfNotExist(networkType networkv1alpha1.NetworkType, cn *networkv1alpha1.ClusterNetwork,
	nodeClient v1.NodeClient, nodenetworkCache ctlnetworkv1alpha1.NodeNetworkCache,
	nodenetworkClient ctlnetworkv1alpha1.NodeNetworkClient) error {

	nodes, err := nodeClient.List(metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes failed, error: %w", err)
	}

	for _, node := range nodes.Items {
		if _, err := CreateNodeNetworkIfNotExist(&node, networkType, cn, nodenetworkCache, nodenetworkClient); err != nil {
			return fmt.Errorf("create nodenetwork CR for node %s failed, error: %w", node.Name, err)
		}
	}

	return nil
}
