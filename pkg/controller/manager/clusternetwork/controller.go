package clusternetwork

import (
	"context"
	"fmt"

	corev1 "github.com/rancher/wrangler/pkg/generated/controllers/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/controller/common"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
)

// ClusterNetwork controller watches ClusterNetworks with the same name of network type to create or delete NodeNetwork.
const (
	ControllerName = "harvester-clusternetwork-controller"
)

type Handler struct {
	nodeClient        corev1.NodeClient
	nodeNetworkCache  ctlnetworkv1.NodeNetworkCache
	nodeNetworkClient ctlnetworkv1.NodeNetworkClient
}

func Register(ctx context.Context, management *config.Management) error {
	clusterNetworks := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()
	nodeNetworks := management.HarvesterNetworkFactory.Network().V1beta1().NodeNetwork()
	nodes := management.CoreFactory.Core().V1().Node()

	handler := &Handler{
		nodeClient:        nodes,
		nodeNetworkCache:  nodeNetworks.Cache(),
		nodeNetworkClient: nodeNetworks,
	}

	if err := initClusterNetwork(clusterNetworks); err != nil {
		return fmt.Errorf("init clusternetwork faield, error: %w", err)
	}

	clusterNetworks.OnChange(ctx, ControllerName, handler.OnChange)
	clusterNetworks.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

func (h Handler) OnChange(key string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn == nil || cn.DeletionTimestamp != nil {
		return nil, nil
	}

	if cn.Name != string(networkv1.NetworkTypeVLAN) {
		return nil, nil
	}

	klog.Infof("cluster network configuration %s has been changed", cn.Name)

	var nic string
	if cn.Config != nil {
		nic = cn.Config[networkv1.KeyDefaultInterface]
	}

	if err := common.SetNICForAllNodeNetworks(cn.Enable, nic, h.nodeNetworkCache, h.nodeNetworkClient); err != nil {
		return nil, err
	}

	return cn, nil
}

func (h Handler) OnRemove(key string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn.Name != string(networkv1.NetworkTypeVLAN) {
		return nil, nil
	}

	klog.Infof("cluster network configuration %s has been deleted", cn.Name)

	if err := common.DeleteAllNodeNetwork(networkv1.NetworkTypeVLAN, h.nodeNetworkCache, h.nodeNetworkClient); err != nil {
		return nil, fmt.Errorf("delete all nodenetwork CRs failed, error: %w", err)
	}

	return cn, nil
}

func initClusterNetwork(client ctlnetworkv1.ClusterNetworkClient) error {
	name := string(networkv1.NetworkTypeVLAN)
	if _, err := client.Get(name, metav1.GetOptions{}); err == nil || !apierrors.IsNotFound(err) {
		return err
	}

	var cn networkv1.ClusterNetwork
	if _, err := client.Create(networkv1.NewClusterNetwork("", name, cn)); err != nil {
		return fmt.Errorf("create clusternetwork failed, error: %w", err)
	}

	return nil
}
