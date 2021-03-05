package node

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	networkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/apis/network.harvester.cattle.io/v1alpha1"
	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/common"
	ctlnetworkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
)

// Harvester network node controller watches node to create or delete NodeNetwork CR
const (
	controllerName = "harvester-network-node-controller"
)

type Handler struct {
	nodeNetworkClient   ctlnetworkv1alpha1.NodeNetworkClient
	nodeNetworkCache    ctlnetworkv1alpha1.NodeNetworkCache
	clusterNetworkCache ctlnetworkv1alpha1.ClusterNetworkCache
}

func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	nodeNetworks := management.HarvesterNetworkFactory.Network().V1alpha1().NodeNetwork()
	clusterNetworks := management.HarvesterNetworkFactory.Network().V1alpha1().ClusterNetwork()

	handler := &Handler{
		nodeNetworkClient:   nodeNetworks,
		nodeNetworkCache:    nodeNetworks.Cache(),
		clusterNetworkCache: clusterNetworks.Cache(),
	}

	nodes.OnChange(ctx, controllerName, handler.OnChange)

	return nil
}

func (h Handler) OnChange(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return nil, nil
	}

	// TODO adjust the definition positions of namespace and name, maybe defined in CRD
	cns, err := h.clusterNetworkCache.List(common.Namespace, labels.Everything())
	if err != nil {
		return nil, fmt.Errorf("get clusternetwork failed, error: %w", err)
	}

	for _, cn := range cns {
		if cn.Name == string(networkv1alpha1.NetworkTypeVLAN) && cn.Enable {
			if _, err := common.CreateNodeNetworkIfNotExist(node, networkv1alpha1.NetworkTypeVLAN, cn,
				h.nodeNetworkCache, h.nodeNetworkClient); err != nil {
				return nil, fmt.Errorf("create nodenetwork failed, error:%w", err)
			}
		}
	}

	return node, nil
}
