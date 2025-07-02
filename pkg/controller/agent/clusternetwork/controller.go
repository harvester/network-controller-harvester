package clusternetwork

import (
	"context"
	"errors"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"github.com/vishvananda/netlink"

	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/network/vlan"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	controllerName = "harvester-network-cn-controller"
)

type Handler struct {
	cnCache   ctlnetworkv1.ClusterNetworkCache
	cnClient  ctlnetworkv1.ClusterNetworkClient
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
	nadClient ctlcniv1.NetworkAttachmentDefinitionClient
}

func Register(ctx context.Context, management *config.Management) error {
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	handler := Handler{
		cnCache:   cns.Cache(),
		cnClient:  cns,
		nadClient: nads,
		nadCache:  nads.Cache(),
	}

	cns.OnChange(ctx, controllerName, handler.OnChange)
	return nil
}

// to support vlan trunk mode nad
// the vlan set of a specific cluster network is computed dynamically via the nad list
func (h Handler) OnChange(_ string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn == nil || cn.DeletionTimestamp != nil {
		return nil, nil
	}
	klog.Infof("cluster network %s has been changed, vid hash: %v", cn.Name, cn.Annotations[utils.KeyVlanIDSetStrHash])

	v, err := vlan.GetVlan(cn.Name)
	if err != nil {
		// vlanconfig controller sets up the non-mgmt cn; mgmt cn is setup by wicked daemon service
		if errors.As(err, &netlink.LinkNotFoundError{}) {
			klog.Infof("cluster network %s is not set on this node, skip", cn.Name)
			return nil, nil
		}
		return nil, err
	}

	cnVlans, err := utils.GeVlanIDSetFromClusterNetwork(cn.Name, h.nadCache)
	if err != nil {
		klog.Infof("cluster network %s failed to get vlanset %s", cn.Name, err.Error())
		return nil, err
	}

	// user might configure vlan sub-interface on the existing bridge for additional usage
	// those vids are out of any nads
	// try to keep them as more as possbile, and log errors if failed to list or add
	manualVlans, err := iface.GetManuallyConfiguredVlans(cn.Name)
	if err != nil {
		klog.Infof("cluster network %s failed to get manually configured vlans from link, error: %s, skip", cn.Name, err.Error())
	} else {
		for i := range manualVlans {
			err := cnVlans.SetUint16VID(manualVlans[i])
			if err != nil {
				klog.Infof("cluster network %s failed to add the manually configured vid %v to vlanset, error %s, skip", cn.Name, manualVlans[i], err.Error())
				continue
			}
		}
	}

	// get current set vlan
	existingVlans, err := v.ToVlanIDSet()
	if err != nil {
		return nil, err
	}
	added, removed, err := cnVlans.Diff(existingVlans)
	if err != nil {
		return nil, err
	}
	klog.Infof("cluster network %s will add %v vlans, remove %v vlans", cn.Name, added.GetVlanCount(), removed.GetVlanCount())

	err = v.AddLocalAreas(added)
	if err != nil {
		return nil, err
	}
	err = v.RemoveLocalAreas(removed)
	if err != nil {
		return nil, err
	}

	return cn, nil
}
