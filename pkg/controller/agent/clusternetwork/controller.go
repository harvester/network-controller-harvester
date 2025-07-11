package clusternetwork

import (
	"context"
	"fmt"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"

	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"

	"github.com/harvester/harvester-network-controller/pkg/network/iface"
)

const (
	controllerName = "harvester-network-cn-controller"
)

type Handler struct {
	cnCache   ctlnetworkv1.ClusterNetworkClient
	cnClient  ctlnetworkv1.ClusterNetworkClient
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
	nadClient ctlcniv1.NetworkAttachmentDefinitionClient
	mgmtVlan  int
}

func Register(ctx context.Context, management *config.Management) error {
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()

	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()

	h := Handler{
		cnCache:   cns.Cache(),
		cnClient:  cns,
		nadClient: nads,
		nadCache:  nads.Cache(),
	}

	vlanID, err := iface.GetMgmtVlan()
	if err != nil {
		return fmt.Errorf("failed to get mgmt vlan err:%v", err)
	}
	handler.mgmtVlan = vlanID

	cns.OnChange(ctx, controllerName, h.OnChange)

	return nil
}

func (h Handler) OnChange(_ string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn == nil || cn.DeletionTimestamp != nil {
		return nil, nil
	}
	klog.Debugf("cluster network %s has been changed, vid hash: %+v", cn.Name, cn.Name)

	v, err = vlan.GetVlan(cn.Name)
	if err != nil {
		// vlanconfig controller sets up the non-mgmt cn; mgmt cn is setup by wicked daemon service
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			klog.Debugf("cluster network %s is not set on this node, skip", cn.Name)
			return nil
		}
		return err
	}

	cnVlans, err := utils.GeVlanIdSetFromClusterNetwork(cn.Name, h.nadCache)
	if err != nil {
		klog.Infof("cluster network %s failed to get vlanset %s", cn.Name, err.Errorf())
		return err
	}

	// if mgmt network, add mgmvid to list

	if utils.IsManagementClusterNetwork(cn.Name) {
		cnVlans.SetVid(h.mgmtVlan)
	}

	// get current set vlan, mgmt vid is always there
	existingVlans, err := v.ToVlanIDSet()
	if err != nil {
		return err
	}

	// get diff list, to add, to remove

	return cn, nil
}
