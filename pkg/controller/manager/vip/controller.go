package vip

import (
	"context"
	"fmt"

	ctlappsv1 "github.com/rancher/wrangler/pkg/generated/controllers/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	"github.com/harvester/harvester-network-controller/pkg/network/mgmt"
	"github.com/harvester/harvester-network-controller/pkg/network/vlan"
)

const (
	ControllerName = "harvester-vip-controller"

	vipNamespace        = "harvester-system"
	vipWorkloadName     = "kube-vip"
	vipContainerName    = "kube-vip"
	vipInterfaceEnvName = "vip_interface"
)

type Handler struct {
	dsCtr   ctlappsv1.DaemonSetController
	dsCache ctlappsv1.DaemonSetCache
	mgmtNIC string
}

func Register(ctx context.Context, management *config.Management) error {
	ds := management.AppsFactory.Apps().V1().DaemonSet()
	nodeNetworks := management.HarvesterNetworkFactory.Network().V1beta1().NodeNetwork()

	handler := &Handler{
		dsCtr:   ds,
		dsCache: ds.Cache(),
	}

	// The manager network controller and kube-vip should deployed in the same node to make sure they get the same management network NIC.
	mgmtNetwork, err := mgmt.NewFlannelNetwork(management.Options.MgmtNetworkDevice)
	if err != nil {
		return fmt.Errorf("new flannel network failed, error: %w", err)
	}
	handler.mgmtNIC = mgmtNetwork.NIC().Name()

	nodeNetworks.OnChange(ctx, ControllerName, handler.OnChange)
	nodeNetworks.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

func (h Handler) OnChange(key string, nn *networkv1.NodeNetwork) (*networkv1.NodeNetwork, error) {
	if nn == nil || nn.DeletionTimestamp != nil {
		return nil, nil
	}

	if nn.Spec.Type != networkv1.NetworkTypeVLAN {
		return nil, nil
	}

	var readyStatus, removedStatus bool
	for _, con := range nn.Status.Conditions {
		if con.Type == networkv1.NodeNetworkReady {
			readyStatus = con.Status == corev1.ConditionTrue
		}
		if con.Type == networkv1.NodeNetworkRemoved {
			removedStatus = con.Status == corev1.ConditionTrue
		}
	}

	// The network has been ready
	if readyStatus && !removedStatus {
		return nn, h.updateVipInterface(nn.Spec.NIC)
	}
	if removedStatus {
		return nn, h.updateVipInterface(vlan.BridgeName)
	}

	return nn, nil
}

func (h Handler) OnRemove(key string, nn *networkv1.NodeNetwork) (*networkv1.NodeNetwork, error) {
	if nn == nil {
		return nil, nil
	}
	klog.Infof("networkNode %s is removed", nn.Name)

	if nn.Spec.Type != networkv1.NetworkTypeVLAN {
		return nn, nil
	}

	for _, con := range nn.Status.Conditions {
		if con.Type == networkv1.NodeNetworkRemoved {
			if con.Status == corev1.ConditionTrue {
				return nn, h.updateVipInterface(vlan.BridgeName)
			}
			return nil, fmt.Errorf("the vlan network hasn't removed")
		}
	}

	return nil, fmt.Errorf("the vlan network hasn't removed")
}

// The NIC used by the VLAN network is no longer available for the kube-vip. Thus, we have to modify the vip interface to harvester-br0.
// If the NIC is released from the VLAN network and the harvester-br0 is down, we can modify the vip interface to the default NIC.
func (h Handler) updateVipInterface(unavailableNIC string) error {
	klog.Infof("update vip interface, unavailable NIC: %s", unavailableNIC)
	ds, err := h.dsCache.Get(vipNamespace, vipWorkloadName)
	if err != nil {
		return fmt.Errorf("get kube-vip daemonset cache failed, error: %w", err)
	}

	var vipInterface string
	var containerIndex, envIndex int
	for i, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == vipContainerName {
			containerIndex = i
			for j, env := range c.Env {
				if env.Name == vipInterfaceEnvName {
					envIndex = j
					vipInterface = env.Value
					break
				}
			}
			break
		}
	}

	if vipInterface != unavailableNIC {
		return nil
	}

	dsCopy := ds.DeepCopy()
	if vipInterface == vlan.BridgeName {

		dsCopy.Spec.Template.Spec.Containers[containerIndex].Env[envIndex].Value = h.mgmtNIC
	} else {
		dsCopy.Spec.Template.Spec.Containers[containerIndex].Env[envIndex].Value = vlan.BridgeName
	}

	if _, err := h.dsCtr.Update(dsCopy); err != nil {
		return fmt.Errorf("update kube-vip daemonset failed, error: %w", err)
	}

	return nil
}
