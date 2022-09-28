package vlanconfig

import (
	"context"
	"fmt"
	"github.com/cenk/backoff"
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
	vsCache  ctlnetworkv1.VlanStatusCache
}

func Register(ctx context.Context, management *config.Management) error {
	vcs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()
	vss := management.HarvesterNetworkFactory.Network().V1beta1().VlanStatus()
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()

	handler := &Handler{
		cnClient: cns,
		cnCache:  cns.Cache(),
		vsCache:  vss.Cache(),
	}

	if err := handler.initialize(); err != nil {
		return fmt.Errorf("initialize error: %w", err)
	}

	vcs.OnChange(ctx, ControllerName, handler.EnsureClusterNetwork)
	vss.OnChange(ctx, ControllerName, handler.SetClusterNetworkReady)
	vss.OnRemove(ctx, ControllerName, handler.SetClusterNetworkUnready)

	return nil
}

func (h Handler) EnsureClusterNetwork(key string, vc *networkv1.VlanConfig) (*networkv1.VlanConfig, error) {
	if vc == nil || vc.DeletionTimestamp != nil {
		return nil, nil
	}

	klog.Infof("vlan config %s has been changed, spec: %+v", vc.Name, vc.Spec)

	if err := h.ensureClusterNetwork(vc.Spec.ClusterNetwork); err != nil {
		return nil, err
	}
	return vc, nil
}

func (h Handler) SetClusterNetworkReady(key string, vs *networkv1.VlanStatus) (*networkv1.VlanStatus, error) {
	if vs == nil || vs.DeletionTimestamp != nil {
		return nil, nil
	}

	klog.Infof("vlan status %s has been changed, node: %s, clusterNetwork: %s, vc: %s", vs.Name, vs.Status.Node,
		vs.Status.ClusterNetwork, vs.Status.VlanConfig)

	if err := h.setClusterNetworkReady(vs); err != nil {
		return nil, fmt.Errorf("set cluster network of vs %s ready failed, error: %w", vs.Name, err)
	}

	return vs, nil
}

func (h Handler) SetClusterNetworkUnready(key string, vs *networkv1.VlanStatus) (*networkv1.VlanStatus, error) {
	if vs == nil {
		return nil, nil
	}

	if err := h.setClusterNetworkUnready(vs); err != nil {
		return nil, fmt.Errorf("set cluster network unready before deleting vs %s failed, error: %w", vs.Name, err)
	}

	return vs, nil
}

func (h Handler) ensureClusterNetwork(name string) error {
	if _, err := h.cnCache.Get(name); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil {
		return nil
	}

	// if cn is not existing
	cn := &networkv1.ClusterNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if _, err := h.cnClient.Create(cn); err != nil {
		return err
	}

	return nil
}

func (h Handler) setClusterNetworkReady(vs *networkv1.VlanStatus) error {
	cn, err := h.cnCache.Get(vs.Status.ClusterNetwork)
	if err != nil {
		return err
	}

	if networkv1.Ready.IsTrue(cn.Status) {
		return nil
	}
	cnCopy := cn.DeepCopy()
	networkv1.Ready.True(&cnCopy.Status)
	if _, err := h.cnClient.Update(cnCopy); err != nil {
		return err
	}

	return nil
}

func (h Handler) setClusterNetworkUnready(vs *networkv1.VlanStatus) error {
	vsList, err := h.vsCache.List(labels.Set{
		utils.KeyClusterNetworkLabel: vs.Status.ClusterNetwork,
	}.AsSelector())
	if err != nil {
		return err
	}
	if len(vsList) > 1 {
		return nil
	}
	if len(vsList) == 1 && vsList[0].Name != vs.Name {
		return fmt.Errorf("the only remain vlanstatus %s is not %s", vsList[0].Name, vs.Name)
	}

	// Only remain this vlanstatus being deleted
	cn, err := h.cnCache.Get(vs.Status.ClusterNetwork)
	if err != nil {
		return err
	}
	if networkv1.Ready.IsFalse(cn.Status) {
		return nil
	}
	cnCopy := cn.DeepCopy()
	networkv1.Ready.False(&cnCopy.Status)
	if _, err := h.cnClient.Update(cnCopy); err != nil {
		return err
	}

	return nil
}

func (h Handler) initialize() error {
	if err := backoff.Retry(func() error {
		// It's not allowed to use the local cache to get the cluster network in the register period
		// because the factory hasn't started. We just create the cluster network and ignore the `AlreadyExists` error.
		mgmtCn := &networkv1.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{
				Name: utils.ManagementClusterNetworkName,
			},
		}
		networkv1.Ready.True(&mgmtCn.Status)
		if _, err := h.cnClient.Create(mgmtCn); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create %s failed, error: %w", utils.ManagementClusterNetworkName, err)
		}
		return nil
	}, backoff.NewExponentialBackOff()); err != nil {
		return err
	}

	return nil
}
