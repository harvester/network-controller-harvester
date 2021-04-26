package common

import (
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
)

const (
	Namespace        = "harvester-system"
	KeyNodeName      = "NODENAME"
	KeyLabelNodeName = "network.harvesterhci.io/nodename"
	initStatusMsg    = "Initializing network configuration"
)

func NewNodeNetworkFromNode(node *corev1.Node, networkType networkv1.NetworkType,
	cn *networkv1.ClusterNetwork) *networkv1.NodeNetwork {
	nn := &networkv1.NodeNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      node.Name + "-" + string(networkType),
			Namespace: Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "ctlcorev1",
					Kind:       "Node",
					Name:       node.Name,
					UID:        node.UID,
				},
			},
			Labels: map[string]string{
				KeyLabelNodeName: node.Name,
			},
		},
		Spec: networkv1.NodeNetworkSpec{
			Type:     networkType,
			NodeName: node.Name,
		},
		Status: networkv1.NodeNetworkStatus{
			NetworkIDs:        []networkv1.NetworkID{},
			NetworkLinkStatus: map[string]*networkv1.LinkStatus{},
			Conditions:        []networkv1.Condition{},
		},
	}

	// initialize status
	networkv1.NodeNetworkReady.SetStatusBool(nn, false)
	networkv1.NodeNetworkReady.Message(nn, initStatusMsg)

	switch networkType {
	case networkv1.NetworkTypeVLAN:
		defaultPhysicalNIC, ok := cn.Config[networkv1.KeyDefaultNIC]
		if ok {
			nn.Spec.NIC = defaultPhysicalNIC
		}
	default:
	}

	return nn
}

func CreateNodeNetworkIfNotExist(node *corev1.Node, networkTye networkv1.NetworkType,
	cn *networkv1.ClusterNetwork, nodeNetworkCache ctlnetworkv1.NodeNetworkCache,
	nodeNetworkClient ctlnetworkv1.NodeNetworkClient) (*networkv1.NodeNetwork, error) {
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

func DeleteAllNodeNetwork(networkTye networkv1.NetworkType, nodeNetworkCache ctlnetworkv1.NodeNetworkCache,
	nodeNetworkClient ctlnetworkv1.NodeNetworkClient) error {
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

func CreateAllNodeNetworkIfNotExist(networkType networkv1.NetworkType, cn *networkv1.ClusterNetwork,
	nodeClient ctlcorev1.NodeClient, nodenetworkCache ctlnetworkv1.NodeNetworkCache,
	nodenetworkClient ctlnetworkv1.NodeNetworkClient) error {

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
