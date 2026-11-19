package clusternetwork

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/cenk/backoff"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/iface"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
)

const (
	controllerName = "harvester-network-manager-cn-controller"
	nicMonitorName = "nic"
)

type Handler struct {
	lmClient          ctlnetworkv1.LinkMonitorClient
	lmCache           ctlnetworkv1.LinkMonitorCache
	cnClient          ctlnetworkv1.ClusterNetworkClient
	nadClient         ctlcniv1.NetworkAttachmentDefinitionClient
	nadCache          ctlcniv1.NetworkAttachmentDefinitionCache
	hostNetworkCache  ctlnetworkv1.HostNetworkConfigCache
	hostNetworkClient ctlnetworkv1.HostNetworkConfigClient
	nodeCache         ctlcorev1.NodeCache
	vcCache           ctlnetworkv1.VlanConfigCache
	cnController      ctlnetworkv1.ClusterNetworkController
}

func Register(ctx context.Context, management *config.Management) error {
	cns := management.HarvesterNetworkFactory.Network().V1beta1().ClusterNetwork()
	lms := management.HarvesterNetworkFactory.Network().V1beta1().LinkMonitor()
	nads := management.CniFactory.K8s().V1().NetworkAttachmentDefinition()
	hns := management.HarvesterNetworkFactory.Network().V1beta1().HostNetworkConfig()
	nodes := management.CoreFactory.Core().V1().Node()
	vcs := management.HarvesterNetworkFactory.Network().V1beta1().VlanConfig()

	h := Handler{
		lmClient:          lms,
		lmCache:           lms.Cache(),
		cnClient:          cns,
		nadClient:         nads,
		nadCache:          nads.Cache(),
		hostNetworkCache:  hns.Cache(),
		hostNetworkClient: hns,
		nodeCache:         nodes.Cache(),
		vcCache:           vcs.Cache(),
	}

	if err := h.initialize(); err != nil {
		return fmt.Errorf("initialize error: %w", err)
	}

	cns.OnChange(ctx, controllerName, h.EnsureLinkMonitor)
	cns.OnChange(ctx, controllerName, h.SetNadReadyLabel)
	cns.OnChange(ctx, controllerName, h.SetHostNetworkStatus)
	cns.OnRemove(ctx, controllerName, h.DeleteLinkMonitor)
	// the link monitor of an isolated cluster network lists the VlanConfig NICs, so re-check it when they change
	h.cnController = cns
	vcs.OnChange(ctx, controllerName+"-vc", h.EnqueueClusterNetworkOfVlanConfig)

	return nil
}

// EnqueueClusterNetworkOfVlanConfig re-checks the cluster network of a changed VlanConfig, e.g. to refresh the
// link monitor rule of an isolated cluster network when its NICs change
func (h Handler) EnqueueClusterNetworkOfVlanConfig(_ string, vc *networkv1.VlanConfig) (*networkv1.VlanConfig, error) {
	if vc != nil && vc.Spec.ClusterNetwork != "" {
		h.cnController.Enqueue(vc.Spec.ClusterNetwork)
	}
	return vc, nil
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
	nameRule, err := h.linkMonitorNameRule(name)
	if err != nil {
		return err
	}

	lm, err := h.lmCache.Get(name)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		if lm.Spec.TargetLinkRule.NameRule == nameRule {
			return nil
		}
		lmCopy := lm.DeepCopy()
		lmCopy.Spec.TargetLinkRule.NameRule = nameRule
		if _, err := h.lmClient.Update(lmCopy); err != nil {
			return err
		}
		logrus.Infof("link monitor %s name rule updated to %s", name, nameRule)
		return nil
	}

	if _, err := h.lmClient.Create(&networkv1.LinkMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: networkv1.LinkMonitorSpec{
			TargetLinkRule: networkv1.TargetLinkRule{
				NameRule: nameRule,
			}},
	}); err != nil {
		return err
	}

	return nil
}

// linkMonitorNameRule matches the bridge and bond of the cluster network. With isolated uplinks there is no bond and
// every NIC is its own uplink, so the NICs of the cluster network's VlanConfigs are matched too, to report the
// state of each link.
func (h Handler) linkMonitorNameRule(name string) (string, error) {
	rule := name + "(" + utils.BridgeSuffix + "|" + utils.BondSuffix + ")"

	isolated, err := utils.IsIsolatedClusterNetwork(name, h.vcCache)
	if err != nil || !isolated {
		return rule, err
	}

	vcs, err := h.vcCache.List(labels.Set{utils.KeyClusterNetworkLabel: name}.AsSelector())
	if err != nil {
		return "", err
	}
	nics := make(map[string]bool)
	for _, vc := range vcs {
		if vc.DeletionTimestamp != nil {
			continue
		}
		for _, nic := range vc.Spec.Uplink.NICs {
			nics[regexp.QuoteMeta(nic)] = true
		}
	}
	if len(nics) == 0 {
		return rule, nil
	}
	names := make([]string, 0, len(nics))
	for nic := range nics {
		names = append(names, nic)
	}
	sort.Strings(names)

	return "^" + rule + "$|^(" + strings.Join(names, "|") + ")$", nil
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
		if nad.DeletionTimestamp != nil {
			continue
		}
		if utils.GetNadLabel(nad, utils.KeyNetworkReady) == isReady {
			continue
		}
		nadCopy := nad.DeepCopy()
		utils.SetNadLabel(nadCopy, utils.KeyNetworkReady, isReady)
		if _, err := h.nadClient.Update(nadCopy); err != nil {
			return err
		}
	}

	return nil
}

func (h Handler) setHNNodeStatusUnready(hnCopy *networkv1.HostNetworkConfig) error {
	nodes, err := h.nodeCache.List(labels.Everything())
	if err != nil {
		return err
	}

	for _, node := range nodes {
		if node.DeletionTimestamp != nil {
			continue
		}

		if hnCopy.Status.NodeStatus == nil {
			continue
		}

		if _, exists := hnCopy.Status.NodeStatus[node.Name]; !exists {
			continue
		}

		patchPayload := map[string]interface{}{
			"status": map[string]interface{}{
				"nodeStatus": map[string]interface{}{
					node.Name: map[string]interface{}{
						"clusterNetwork": hnCopy.Spec.ClusterNetwork,
						"vlanID":         hnCopy.Spec.VlanID,
						"mode":           hnCopy.Spec.Mode,
						"conditions": []map[string]interface{}{
							{
								"type":    networkv1.Ready,
								"status":  "False",
								"message": "cluster network is down, setup l3 connectivity failed",
							},
						},
					},
				},
			},
		}

		patchBytes, err := json.Marshal(patchPayload)
		if err != nil {
			return fmt.Errorf("failed to marshal patch: %w", err)
		}

		_, err = h.hostNetworkClient.Patch(
			hnCopy.Name,
			types.MergePatchType,
			patchBytes,
			"status",
		)

		if err != nil {
			return fmt.Errorf("failed to patch HostNetworkConfig status for node %s: %w", node.Name, err)
		}
	}

	return nil
}

func (h Handler) SetHostNetworkStatus(_ string, cn *networkv1.ClusterNetwork) (*networkv1.ClusterNetwork, error) {
	if cn == nil {
		return nil, nil
	}

	if err := h.setHostNetworkStatus(cn); err != nil {
		return nil, err
	}

	return cn, nil
}

func (h Handler) setHostNetworkStatus(cn *networkv1.ClusterNetwork) error {
	isReady := utils.ValueFalse

	if cn.DeletionTimestamp == nil && networkv1.Ready.IsTrue(cn.Status) {
		isReady = utils.ValueTrue
	}

	hostnetworkconfigs, err := h.hostNetworkCache.List(labels.Everything())
	if err != nil {
		return err
	}

	updateFunc := h.setHostNetworkUnready
	if isReady == utils.ValueTrue {
		updateFunc = h.setHostNetworkReady
	}

	for _, hostnetworkconfig := range hostnetworkconfigs {
		if hostnetworkconfig.Spec.ClusterNetwork != cn.Name {
			continue
		}

		if hostnetworkconfig.DeletionTimestamp != nil {
			continue
		}

		if err := updateFunc(hostnetworkconfig); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) setHostNetworkReady(hn *networkv1.HostNetworkConfig) error {
	if networkv1.Ready.IsTrue(hn.Status) {
		return nil
	}

	hnCopy := hn.DeepCopy()
	networkv1.Ready.True(&hnCopy.Status)
	//empty message is needed to trigger the status update when the status is already false before
	networkv1.Ready.Message(hnCopy.Status, "")

	//hostnetworkconfig controller will set all hostnetworkconfig per node status to ready
	//set only the global status as ready here
	if _, err := h.hostNetworkClient.Update(hnCopy); err != nil {
		return err
	}

	return nil
}

func (h *Handler) setHostNetworkUnready(hn *networkv1.HostNetworkConfig) error {
	if networkv1.Ready.IsFalse(hn.Status) {
		return nil
	}

	hnCopy := hn.DeepCopy()
	networkv1.Ready.False(&hnCopy.Status)
	networkv1.Ready.Message(hnCopy.Status, "cluster network is down, global host network status is unready")

	//set all hostnetworkconfig per node status to unready
	err := h.setHNNodeStatusUnready(hnCopy)
	if err != nil {
		return err
	}

	return nil
}

func (h Handler) initialize() error {
	return backoff.Retry(func() error {
		if err := h.initializeClusterNetwork(); err != nil {
			logrus.Info(err)
			return err
		}
		if err := h.initializeLinkMonitor(); err != nil {
			logrus.Info(err)
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
