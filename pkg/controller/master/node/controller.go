package node

import (
	"context"

	ctlharv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/common"
	ctlnetworkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
)

const (
	controllerName = "harvester-network-node-controller"
)

type Handler struct {
	hostNetworkClient ctlnetworkv1alpha1.HostNetworkClient
	hostNetworkCache  ctlnetworkv1alpha1.HostNetworkCache
	settingCache      ctlharv1.SettingCache
}

func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	hostNetworks := management.HarvesterNetworkFactory.Network().V1alpha1().HostNetwork()
	settings := management.HarvesterFactory.Harvester().V1alpha1().Setting()

	handler := &Handler{
		hostNetworkClient: hostNetworks,
		hostNetworkCache:  hostNetworks.Cache(),
		settingCache:      settings.Cache(),
	}

	nodes.OnChange(ctx, controllerName, handler.OnChange)

	return nil
}

func (h Handler) OnChange(key string, node *corev1.Node) (*corev1.Node, error) {
	if node == nil || node.DeletionTimestamp != nil {
		return nil, nil
	}
	networkSetting, err := common.GetNetworkSetting(h.settingCache)
	if err != nil {
		return nil, err
	}

	if networkSetting.NIC == "" {
		return node, nil
	}

	if _, err := common.CreateHostNetworkIfNotExist(node, networkSetting, h.hostNetworkCache, h.hostNetworkClient); err != nil {
		return nil, err
	}

	return node, nil
}
