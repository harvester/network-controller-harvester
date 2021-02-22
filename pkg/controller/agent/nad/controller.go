package nad

import (
	"context"
	"fmt"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlharv1 "github.com/rancher/harvester/pkg/generated/controllers/harvester.cattle.io/v1alpha1"
	"k8s.io/klog"

	"github.com/rancher/harvester-network-controller/pkg/config"
	"github.com/rancher/harvester-network-controller/pkg/controller/common"
	"github.com/rancher/harvester-network-controller/pkg/generated/controllers/network.harvester.cattle.io/v1alpha1"
	vlan2 "github.com/rancher/harvester-network-controller/pkg/network/vlan"
)

const (
	controllerName = "harvester-network-nad-controller"
)

type Handler struct {
	hostNetworkCache v1alpha1.HostNetworkCache
	settingCache     ctlharv1.SettingCache
}

func Register(ctx context.Context, management *config.Management) error {
	nad := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	hns := management.HarvesterNetworkFactory.Network().V1alpha1().HostNetwork()
	settings := management.HarvesterFactory.Harvester().V1alpha1().Setting()

	handler := &Handler{
		hostNetworkCache: hns.Cache(),
		settingCache:     settings.Cache(),
	}

	nad.OnChange(ctx, controllerName, handler.OnChange)
	nad.OnRemove(ctx, controllerName, handler.OnRemove)

	return nil
}

func (h Handler) OnChange(key string, nad *nadv1.NetworkAttachmentDefinition) (*nadv1.NetworkAttachmentDefinition, error) {
	if nad == nil || nad.DeletionTimestamp != nil {
		return nil, nil
	}

	if nad.Spec.Config == "" {
		return nad, nil
	}

	klog.Infof("nad configuration %s has been changed: %s", nad.Name, nad.Spec.Config)
	netconf, err := common.DecodeNetConf(nad.Spec.Config)
	if err != nil {
		return nil, err
	}

	// TODO delete previous vlan id when update nad

	nic, err := common.GetNIC(h.hostNetworkCache, h.settingCache)
	if err != nil {
		return nil, fmt.Errorf("get nic failed, error: %w", err)
	}

	vlan, err := vlan2.NewVlanWithNic(nic, nil)
	if err != nil {
		return nil, err
	}

	if err := vlan.AddLocalArea(netconf.Vlan); err != nil {
		return nil, err
	}

	return nad, nil
}

func (h Handler) OnRemove(key string, nad *nadv1.NetworkAttachmentDefinition) (*nadv1.NetworkAttachmentDefinition, error) {
	if nad == nil {
		return nil, nil
	}

	if nad.Spec.Config == "" {
		return nad, nil
	}

	klog.Infof("nad configuration %s has been deleted.", nad.Name)
	netconf, err := common.DecodeNetConf(nad.Spec.Config)
	if err != nil {
		return nil, err
	}

	nic, err := common.GetNIC(h.hostNetworkCache, h.settingCache)
	if err != nil {
		return nil, fmt.Errorf("get nic failed, error: %w", err)
	}

	vlan, err := vlan2.NewVlanWithNic(nic, nil)
	if err != nil {
		return nil, err
	}

	if err := vlan.RemoveLocalArea(netconf.Vlan); err != nil {
		return nil, err
	}

	return nad, nil
}
