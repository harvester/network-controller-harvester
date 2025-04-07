package linkmonitor

import (
	"context"
	"fmt"

	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"github.com/vishvananda/netlink"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/config"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/network/monitor"
)

const (
	controllerName = "harvester-network-link-monitor-controller"
)

type Handler struct {
	nodeName string

	nodeCache    ctlcorev1.NodeCache
	lmController ctlnetworkv1.LinkMonitorController
	lmClient     ctlnetworkv1.LinkMonitorClient

	linkMonitor *monitor.Monitor
}

func Register(ctx context.Context, management *config.Management) error {
	lms := management.HarvesterNetworkFactory.Network().V1beta1().LinkMonitor()
	nodes := management.CoreFactory.Core().V1().Node()

	h := &Handler{
		nodeName:     management.Options.NodeName,
		nodeCache:    nodes.Cache(),
		lmController: lms,
		lmClient:     lms,
	}

	// initial and start link monitor
	h.linkMonitor = monitor.NewMonitor(&monitor.Handler{
		NewLink: h.UpdateLink,
		DelLink: h.UpdateLink,
	})
	go h.linkMonitor.Start(ctx)

	lms.OnChange(ctx, controllerName, h.OnChange)
	lms.OnRemove(ctx, controllerName, h.OnRemove)

	return nil
}

func (h Handler) OnChange(_ string, lm *networkv1.LinkMonitor) (*networkv1.LinkMonitor, error) {
	if lm == nil || lm.DeletionTimestamp != nil {
		return nil, nil
	}

	klog.V(5).Infof("link monitor %s has been changed, spec: %+v", lm.Name, lm.Spec)

	isMatch, err := h.isMatchCurrentNode(lm)
	if err != nil {
		return nil, err
	}
	if !isMatch {
		h.DeletePattern(lm)
		return lm, nil
	}

	if h.isRuleChange(lm) {
		h.AddPattern(lm)
	}

	if err := h.syncLinkStatus(lm); err != nil {
		return nil, fmt.Errorf("synchronize links to link monitor %s failed, error: %w", lm.Name, err)
	}

	return lm, nil
}

func (h Handler) OnRemove(_ string, lm *networkv1.LinkMonitor) (*networkv1.LinkMonitor, error) {
	if lm == nil {
		return nil, nil
	}

	klog.V(5).Infof("link monitor %s has been removed", lm.Name)

	h.DeletePattern(lm)

	return lm, nil
}

func (h Handler) isMatchCurrentNode(lm *networkv1.LinkMonitor) (bool, error) {
	nodes, err := h.nodeCache.List(labels.Set(lm.Spec.NodeSelector).AsSelector())
	if err != nil {
		return false, err
	}

	for _, node := range nodes {
		// ignore the node to be deleted
		if node.Name == h.nodeName && node.DeletionTimestamp == nil {
			klog.Infof("lm %s matches node %s", lm.Name, node.Name)
			return true, nil
		}
	}

	return false, nil
}

func (h Handler) UpdateLink(key string, _ *netlink.LinkUpdate) error {
	h.lmController.Enqueue(key)
	return nil
}

func (h Handler) AddPattern(lm *networkv1.LinkMonitor) {
	pattern := monitor.NewPattern(lm.Spec.TargetLinkRule.TypeRule, lm.Spec.TargetLinkRule.NameRule)

	h.linkMonitor.AddPattern(lm.Name, pattern)
}

func (h Handler) DeletePattern(lm *networkv1.LinkMonitor) {
	h.linkMonitor.DeletePattern(lm.Name)
}

func (h Handler) isRuleChange(lm *networkv1.LinkMonitor) bool {
	pattern := h.linkMonitor.GetPattern(lm.Name)
	if pattern != nil {
		if pattern.Type == lm.Spec.TargetLinkRule.TypeRule && pattern.Name == lm.Spec.TargetLinkRule.NameRule {
			return false
		}
	}

	return true
}

func linkToLinkStatus(l netlink.Link) networkv1.LinkStatus {
	linkStatus := networkv1.LinkStatus{
		Name:        l.Attrs().Name,
		Index:       l.Attrs().Index,
		Type:        l.Type(),
		MAC:         l.Attrs().HardwareAddr.String(),
		Promiscuous: l.Attrs().Promisc != 0,
		MasterIndex: l.Attrs().MasterIndex,
	}

	if l.Attrs().OperState == netlink.OperUp {
		linkStatus.State = networkv1.LinkUp
	} else {
		linkStatus.State = networkv1.LinkDown
	}

	return linkStatus
}

func (h Handler) updateStatus(lm *networkv1.LinkMonitor, linkStatusList []networkv1.LinkStatus) error {
	var currentLinkStatusList []networkv1.LinkStatus
	if lm.Status.LinkStatus != nil {
		currentLinkStatusList = lm.Status.LinkStatus[h.nodeName]
	}
	if compareLinkStatusList(currentLinkStatusList, linkStatusList) {
		return nil
	}

	lmCopy := lm.DeepCopy()

	if lm.Status.LinkStatus == nil {
		lmCopy.Status.LinkStatus = make(map[string][]networkv1.LinkStatus)
	}
	lmCopy.Status.LinkStatus[h.nodeName] = linkStatusList

	if _, err := h.lmClient.Update(lmCopy); err != nil {
		return err
	}

	return nil
}

func compareLinkStatusList(m, n []networkv1.LinkStatus) bool {
	if len(m) != len(n) {
		return false
	}

	for i, linkStatus := range m {
		if linkStatus != n[i] {
			return false
		}
	}

	return true
}

func (h Handler) syncLinkStatus(lm *networkv1.LinkMonitor) error {
	pattern := h.linkMonitor.GetPattern(lm.Name)
	links, err := h.linkMonitor.ScanLinks(pattern)
	if err != nil {
		return err
	}
	linkStatusList := make([]networkv1.LinkStatus, len(links))
	for i, link := range links {
		linkStatusList[i] = linkToLinkStatus(link)
	}

	return h.updateStatus(lm, linkStatusList)
}
