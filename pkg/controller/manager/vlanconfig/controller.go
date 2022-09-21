package vlanconfig

import (
	"context"
	"fmt"
	"github.com/cenk/backoff"
	"github.com/harvester/harvester-network-controller/pkg/controller/manager/common"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	ControllerName = "harvester-network-manager-vlanconfig-controller"
)

type Handler struct {
	cnClient ctlnetworkv1.ClusterNetworkClient
	cnCache  ctlnetworkv1.ClusterNetworkCache
	vcCache  ctlnetworkv1.VlanConfigCache
}

func Register(ctx context.Context, management *config.Management) error {
	vcs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()

	handler := &Handler{
		cnClient: cns,
		cnCache:  cns.Cache(),
		vcCache:  vcs.Cache(),
	}

	if err := handler.initialize(); err != nil {
		return fmt.Errorf("initialize error: %w", err)
	}

	vcs.OnChange(ctx, ControllerName, handler.OnChange)
	vcs.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

func (h Handler) OnChange(key string, vc *networkv1.VlanConfig) (*networkv1.VlanConfig, error) {
	if vc == nil || vc.DeletionTimestamp != nil {
		return nil, nil
	}

	klog.Infof("vlan config %s has been changed, spec: %+v", vc.Name, vc.Spec)

	if err := h.ensureClusterNetwork(vc.Spec.ClusterNetwork); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h Handler) OnRemove(key string, vc *networkv1.VlanConfig) (*networkv1.VlanConfig, error) {
	klog.Infof("vlan config %s has been removed", vc.Name)
	// delete clusternetwork if there isn't any other vlanconfigs
	if err := h.deleteClusterNetwork(vc.Spec.ClusterNetwork); err != nil {
		return nil, err
	}

	return vc, nil
}

func (h Handler) ensureClusterNetwork(name string) error {
	_, err := h.cnCache.Get(name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		return nil
	}
	// if cn is not existing
	if _, err := h.cnClient.Create(&networkv1.ClusterNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}); err != nil {
		return err
	}

	return nil
}

func (h Handler) deleteClusterNetwork(name string) error {
	vcs, err := h.vcCache.List(labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: name,
	}).AsSelector())
	if err != nil {
		return err
	}

	if len(vcs) != 1 {
		return nil
	}
	return h.cnClient.Delete(name, &metav1.DeleteOptions{})
}

func (h Handler) initialize() error {
	if err := backoff.Retry(func() error {
		// It's not allowed to use the local cache to get the cluster network in the register period
		// because the factory hasn't started. We just create the cluster network and ignore the `AlreadyExists` error.
		if _, err := h.cnClient.Create(&networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: common.ManagementClusterNetworkName,
			},
		}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create %s failed, error: %w", common.ManagementClusterNetworkName, err)
		}
		return nil
	}, backoff.NewExponentialBackOff()); err != nil {
		return err
	}

	return nil
}
