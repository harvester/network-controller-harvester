package clusternetwork

import (
	"context"
	"fmt"

	"github.com/cenk/backoff"
	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	controllerName = "harvester-network-manager-cn-controller"
	nicMonitorName = "nic"
)

type Handler struct {
	lmClient  ctlnetworkv1.LinkMonitorClient
	lmCache   ctlnetworkv1.LinkMonitorCache
	cnClient  ctlnetworkv1.ClusterNetworkClient
	nadClient ctlcniv1.NetworkAttachmentDefinitionClient
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
}

func Register(ctx context.Context, management *config.Management) error {
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()
	lms := management.HarvesterNetworkFactory.Network().V1beta1().LinkMonitor()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()

	h := Handler{
		lmClient:  lms,
		lmCache:   lms.Cache(),
		cnClient:  cns,
		nadClient: nads,
		nadCache:  nads.Cache(),
	}

	if err := h.initialize(); err != nil {
		return fmt.Errorf("initialize error: %w", err)
	}

	cns.OnChange(ctx, controllerName, h.EnsureLinkMonitor)
	cns.OnChange(ctx, controllerName, h.SetNadReadyLabel)
	cns.OnRemove(ctx, controllerName, h.DeleteLinkMonitor)

	return nil
}

func (h Handler) EnsureLinkMonitor(_ string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn == nil || cn.DeletionTimestamp != nil {
		return nil, nil
	}

	if !networkv1.Ready.IsTrue(cn.Status) {
		return cn, nil
	}

	if err := h.ensureLinkMonitor(cn.Name); err != nil {
		return nil, fmt.Errorf("ensure link monitor for cluster network %s failed, error: %w", cn.Name, err)
	}

	return cn, nil
}

func (h Handler) DeleteLinkMonitor(_ string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn == nil {
		return nil, nil
	}

	if err := h.deleteLinkMonitor(cn.Name); err != nil {
		return nil, fmt.Errorf("delete link monitor for cluster network %s failed, error: %w", cn.Name, err)
	}

	return cn, nil
}

func (h Handler) SetNadReadyLabel(_ string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn == nil {
		return nil, nil
	}

	if err := h.setNadReadyLabel(cn); err != nil {
		return nil, err
	}

	return cn, nil
}

func (h Handler) ensureLinkMonitor(name string) error {
	_, err := h.lmCache.Get(name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		return nil
	}

	if _, err := h.lmClient.Create(&networkv1.LinkMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkv1.LinkMonitorSpec{
			TargetLinkRule: networkv1.TargetLinkRule{
				NameRule: name + "(" + iface.BridgeSuffix + "|" + iface.BondSuffix + ")",
			}},
	}); err != nil {
		return err
	}

	return nil
}

func (h Handler) deleteLinkMonitor(name string) error {
	if _, err := h.lmCache.Get(name); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if apierrors.IsNotFound(err) {
		return nil
	}

	return h.lmClient.Delete(name, &metav1.DeleteOptions{})
}

func (h Handler) setNadReadyLabel(cn *networkv1.ClusterNetwork) error {
	isReady := utils.ValueFalse
	// Set all net-attach-defs under the cluster network to be deleted as unready
	if cn.DeletionTimestamp == nil && networkv1.Ready.IsTrue(cn.Status) {
		isReady = utils.ValueTrue
	}

	nads, err := h.nadCache.List("", labels.Set(map[string]string{
		utils.KeyClusterNetworkLabel: cn.Name,
	}).AsSelector())
	if err != nil {
		return err
	}

	for _, nad := range nads {
		if nad.Labels[utils.KeyNetworkReady] == isReady {
			continue
		}
		nadCopy := nad.DeepCopy()
		nadCopy.Labels[utils.KeyNetworkReady] = isReady
		if _, err := h.nadClient.Update(nadCopy); err != nil {
			return err
		}
	}

	return nil
}

func (h Handler) initialize() error {
	return backoff.Retry(func() error {
		if err := h.initializeClusterNetwork(); err != nil {
			klog.V(5).Info(err)
			return err
		}
		if err := h.initializeLinkMonitor(); err != nil {
			klog.V(5).Info(err)
			return err
		}
		return nil
	}, backoff.NewExponentialBackOff())
}

func (h Handler) initializeClusterNetwork() error {
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
}

func (h Handler) initializeLinkMonitor() error {
	nicMonitor := &networkv1.LinkMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: nicMonitorName,
		},
		Spec: networkv1.LinkMonitorSpec{
			TargetLinkRule: networkv1.TargetLinkRule{
				TypeRule: iface.TypeDevice,
			},
		},
	}
	if _, err := h.lmClient.Create(nicMonitor); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create %s failed, error: %w", nicMonitorName, err)
	}

	return nil
}
