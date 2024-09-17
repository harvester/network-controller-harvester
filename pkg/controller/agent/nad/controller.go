package nad

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"

	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	"github.com/harvester/harvester-network-controller/pkg/network/vlan"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const ControllerName = "harvester-network-nad-controller"

type Handler struct {
	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
	nadClient ctlcniv1.NetworkAttachmentDefinitionClient
}

func Register(ctx context.Context, management *config.Management) error {
	nad := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()

	handler := &Handler{
		nadCache:  nad.Cache(),
		nadClient: nad,
	}

	nad.OnChange(ctx, ControllerName, handler.OnChange)
	nad.OnRemove(ctx, ControllerName, handler.OnRemove)

	return nil
}

func (h Handler) OnChange(_ string, nad *nadv1.NetworkAttachmentDefinition) (*nadv1.NetworkAttachmentDefinition, error) {
	if nad == nil || nad.DeletionTimestamp != nil {
		return nil, nil
	}

	klog.Infof("nad configuration %s/%s has been changed: %s", nad.Namespace, nad.Name, nad.Spec.Config)

	// nadCopy could be nil
	nadCopy, err := h.removeOutdatedLocalArea(nad)
	if err != nil {
		return nil, fmt.Errorf("remove outdated local area for nad %s/%s failed, error: %w", nad.Namespace, nad.Name, err)
	}

	// NadCaopy could be nil and nad is readonly here
	if err := h.addLocalArea(nad); err != nil {
		return nil, fmt.Errorf("add local area for nad %s/%s failed, error: %w", nad.Namespace, nad.Name, err)
	}

	// update nad if needed
	if nadCopy != nil {
		if _, err := h.nadClient.Update(nadCopy); err != nil {
			return nil, err
		}
	}

	return nad, nil
}

func (h Handler) OnRemove(_ string, nad *nadv1.NetworkAttachmentDefinition) (*nadv1.NetworkAttachmentDefinition, error) {
	if nad == nil {
		return nil, nil
	}

	klog.Infof("nad configuration %s/%s has been deleted", nad.Namespace, nad.Name)

	if !utils.IsVlanNad(nad) {
		return nad, nil
	}

	localArea, err := GetLocalArea(nad.Labels[utils.KeyVlanLabel], nad.Annotations[utils.KeyNetworkRoute])
	if err != nil {
		return nil, fmt.Errorf("failed to get local area from nad %s/%s, error: %w", nad.Namespace, nad.Name, err)
	}

	if err := h.removeLocalArea(nad.Labels[utils.KeyClusterNetworkLabel], localArea); err != nil {
		return nil, fmt.Errorf("remove local area %+v failed, cluster network: %s, error: %w",
			localArea, nad.Labels[utils.KeyClusterNetworkLabel], err)
	}

	return nad, nil
}

func (h Handler) addLocalArea(nad *nadv1.NetworkAttachmentDefinition) error {
	if !utils.IsVlanNad(nad) {
		return nil
	}

	v, err := vlan.GetVlan(nad.Labels[utils.KeyClusterNetworkLabel])
	if err != nil && !errors.As(err, &netlink.LinkNotFoundError{}) {
		return err
	} else if errors.As(err, &netlink.LinkNotFoundError{}) {
		return nil
	}

	localArea, err := GetLocalArea(nad.Labels[utils.KeyVlanLabel], nad.Annotations[utils.KeyNetworkRoute])
	if err != nil {
		return fmt.Errorf("failed to get local area from nad %s/%s, error: %w", nad.Namespace, nad.Name, err)
	}

	return v.AddLocalArea(localArea)
}

func (h Handler) existDuplicateNad(vlanIDStr, cn string) (bool, error) {
	nads, err := h.nadCache.List("", labels.Set(map[string]string{
		utils.KeyVlanLabel:           vlanIDStr,
		utils.KeyClusterNetworkLabel: cn,
	}).AsSelector())
	if err != nil {
		return false, err
	}

	return len(nads) > 1, nil
}

func (h Handler) removeLocalArea(clusternetwork string, localArea *vlan.LocalArea) error {
	// Skip the case that there are nads with the same cluster network and VLAN id.
	if ok, err := h.existDuplicateNad(strconv.Itoa(int(localArea.Vid)), clusternetwork); err != nil {
		return err
	} else if ok {
		return nil
	}

	v, err := vlan.GetVlan(clusternetwork)
	if err != nil && !errors.As(err, &netlink.LinkNotFoundError{}) {
		return err
	} else if errors.As(err, &netlink.LinkNotFoundError{}) {
		return nil
	}

	return v.RemoveLocalArea(localArea)
}

func (h Handler) removeOutdatedLocalArea(nad *nadv1.NetworkAttachmentDefinition) (*nadv1.NetworkAttachmentDefinition, error) {
	if nad.Labels[utils.KeyLastNetworkType] != string(utils.L2VlanNetwork) ||
		nad.Labels[utils.KeyLastClusterNetworkLabel] == "" && nad.Labels[utils.KeyLastVlanLabel] == "" {
		return nil, nil
	}

	clusternetwork := nad.Labels[utils.KeyLastClusterNetworkLabel]
	if clusternetwork == "" {
		clusternetwork = nad.Labels[utils.KeyClusterNetworkLabel]
	}
	vlanIDStr := nad.Labels[utils.KeyLastVlanLabel]
	if vlanIDStr == "" {
		vlanIDStr = nad.Labels[utils.KeyVlanLabel]
	}

	localArea, err := GetLocalArea(vlanIDStr, nad.Annotations[utils.KeyNetworkRoute])
	if err != nil {
		return nil, fmt.Errorf("failed to get local area from nad %s/%s, error: %w", nad.Namespace, nad.Name, err)
	}

	if err := h.removeLocalArea(clusternetwork, localArea); err != nil {
		return nil, err
	}

	nadCopy := nad.DeepCopy()
	delete(nadCopy.Labels, utils.KeyLastNetworkType)
	delete(nadCopy.Labels, utils.KeyLastClusterNetworkLabel)
	delete(nadCopy.Labels, utils.KeyLastVlanLabel)
	return nadCopy, nil
}

func GetLocalArea(vlanIDStr, routeConf string) (*vlan.LocalArea, error) {
	vlanID, err := strconv.Atoi(vlanIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid vlan id %s", vlanIDStr)
	}

	layer3NetworkConf := &utils.Layer3NetworkConf{}
	if routeConf != "" {
		if layer3NetworkConf, err = utils.NewLayer3NetworkConf(routeConf); err != nil {
			return nil, err
		}
	}

	return &vlan.LocalArea{
		Vid:  uint16(vlanID),
		Cidr: layer3NetworkConf.CIDR,
	}, nil
}
