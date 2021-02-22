package settings

import (
	"context"

	harv1 "github.com/rancher/harvester/pkg/apis/harvester.cattle.io/v1alpha1"
	ctlharv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	ctlcorev1 "github.com/rancher/wrangler-api/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/common"
	ctlnetworkv1alpha1 "github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
)

const (
	controllerName = "harvester-network-settings-controller"
)

type Handler struct {
	NodeClient        ctlcorev1.NodeClient
	NodeCache         ctlcorev1.NodeCache
	settingCache      ctlharv1.SettingCache
	hostNetworkClient ctlnetworkv1alpha1.HostNetworkClient
	hostNetworkCache  ctlnetworkv1alpha1.HostNetworkCache
}

func Register(ctx context.Context, management *config.Management) error {
	nodes := management.CoreFactory.Core().V1().Node()
	hostNetworks := management.HarvesterNetworkFactory.Network().V1alpha1().HostNetwork()
	settings := management.HarvesterFactory.Harvester().V1alpha1().Setting()

	handler := &Handler{
		NodeClient:        nodes,
		NodeCache:         nodes.Cache(),
		hostNetworkClient: hostNetworks,
		hostNetworkCache:  hostNetworks.Cache(),
		settingCache:      settings.Cache(),
	}

	settings.OnChange(ctx, controllerName, handler.OnChange)
	settings.OnRemove(ctx, controllerName, handler.OnChange)

	return nil
}

func (h Handler) OnChange(key string, setting *harv1.Setting) (*harv1.Setting, error) {
	if setting == nil || setting.DeletionTimestamp != nil {
		return nil, nil
	}

	if setting.Name != common.NetworkSettingName {
		return nil, nil
	}

	if setting.Value == "" {
		return nil, nil
	}

	networkSetting, err := common.DecodeNetworkSettings(setting.Value)
	if err != nil {
		return nil, err
	}

	if networkSetting.NIC == "" {
		if err := common.DeleteAllHostNetwork(h.hostNetworkCache, h.hostNetworkClient); err != nil {
			return nil, err
		}
		return setting, nil
	}

	nodeList, err := h.NodeCache.List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for _, node := range nodeList {
		if _, err = common.CreateHostNetworkIfNotExist(node, networkSetting, h.hostNetworkCache, h.hostNetworkClient); err != nil {
			return nil, err
		}
	}

	return setting, nil
}

func (h Handler) OnRemove(key string, setting *harv1.Setting) (*harv1.Setting, error) {
	if setting == nil {
		return nil, nil
	}

	if setting.Name != common.NetworkSettingName {
		return nil, nil
	}

	if err := common.DeleteAllHostNetwork(h.hostNetworkCache, h.hostNetworkClient); err != nil {
		return nil, err
	}

	return setting, nil
}
