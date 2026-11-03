package hostnetworkconfig

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"reflect"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/harvester/webhook/pkg/server/admission"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubevirt.io/v1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
)

const (
	createErr    = "can't create hostnetworkConfig %s because %w"
	updateErr    = "can't update hostnetworkConfig %s because %w"
	deleteErr    = "can't delete hostnetworkConfig %s because %w"
	IPModeDHCP   = "dhcp"
	IPModeStatic = "static"
)

type ValidateOp int

const (
	ValidateCreate ValidateOp = iota
	ValidateUpdate
)

type Validator struct {
	admission.DefaultValidator

	nadCache  ctlcniv1.NetworkAttachmentDefinitionCache
	cnCache   ctlnetworkv1.ClusterNetworkCache
	hncCache  ctlnetworkv1.HostNetworkConfigCache
	vcCache   ctlnetworkv1.VlanConfigCache
	vsCache   ctlnetworkv1.VlanStatusCache
	nodeCache ctlcorev1.NodeCache
	vmCache   ctlkubevirtv1.VirtualMachineCache
}

func NewHostNetworkConfigValidator(
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache,
	cnCache ctlnetworkv1.ClusterNetworkCache,
	hncCache ctlnetworkv1.HostNetworkConfigCache,
	vcCache ctlnetworkv1.VlanConfigCache,
	vsCache ctlnetworkv1.VlanStatusCache,
	nodeCache ctlcorev1.NodeCache,
	vmCache ctlkubevirtv1.VirtualMachineCache,
) *Validator {
	return &Validator{
		nadCache:  nadCache,
		cnCache:   cnCache,
		hncCache:  hncCache,
		vcCache:   vcCache,
		vsCache:   vsCache,
		nodeCache: nodeCache,
		vmCache:   vmCache,
	}
}

var _ admission.Validator = &Validator{}

func (v *Validator) Create(_ *admission.Request, newObj runtime.Object) error {
	hnc := newObj.(*networkv1.HostNetworkConfig)

	if _, err := utils.IsClusterNetworkNameValid(hnc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf(createErr, hnc.Name, err)
	}

	// ensure hostnetwork device name length does not exceed the Linux device name limit of 15 characters.
	if err := utils.IsHostNetworkIntfNameValid(hnc.Spec.ClusterNetwork, hnc.Spec.VlanID); err != nil {
		return fmt.Errorf(createErr, hnc.Name, err)
	}

	// check if clusternetwork exists
	if _, err := v.cnCache.Get(hnc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf(createErr, hnc.Name, fmt.Errorf("it refers to a none-existing cluster network %s or error %w", hnc.Spec.ClusterNetwork, err))
	}

	if err := v.validateHostNetworkConfig(hnc, ValidateCreate); err != nil {
		return fmt.Errorf(createErr, hnc.Name, err)
	}

	if err := v.checkVlanStatusReady(hnc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf(createErr, hnc.Name, err)
	}

	if !hnc.Spec.Underlay {
		return nil
	}
	//vlan interface chosen as underlay must have vlanconfig spanning all nodes
	if err := v.checkVCSpansAllNodes(hnc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf(createErr, hnc.Name, err)
	}

	matchedNode, err := v.nodeSelectorMatchesAllNodes(hnc.Spec.NodeSelector)
	if err != nil {
		return fmt.Errorf(updateErr, hnc.Name, err)
	}
	if !matchedNode {
		return fmt.Errorf(updateErr, hnc.Name, fmt.Errorf("node selector does not match all nodes"))
	}

	return nil
}

func (v *Validator) Update(_ *admission.Request, oldObj, newObj runtime.Object) error {
	oldhnc := oldObj.(*networkv1.HostNetworkConfig)
	newhnc := newObj.(*networkv1.HostNetworkConfig)

	// skip the update if the config has not changed
	if reflect.DeepEqual(newhnc.Spec, oldhnc.Spec) {
		return nil
	}

	if oldhnc.Spec.ClusterNetwork != newhnc.Spec.ClusterNetwork || oldhnc.Spec.VlanID != newhnc.Spec.VlanID {
		return fmt.Errorf(updateErr, newhnc.Name, fmt.Errorf("cannot update clusterNetwork or vlanID field,instead delete the old hostnetworkconfig and create a new one"))
	}

	//do not disable underlay if vms are still using overlay nads
	if oldhnc.Spec.Underlay != newhnc.Spec.Underlay && !newhnc.Spec.Underlay {
		if err := v.checkifVMExistsForOverlayNADs(); err != nil {
			return fmt.Errorf(updateErr, newhnc.Name, err)
		}
	}

	if err := v.validateHostNetworkConfig(newhnc, ValidateUpdate); err != nil {
		return fmt.Errorf(updateErr, newhnc.Name, err)
	}

	if !newhnc.Spec.Underlay {
		return nil
	}

	//vlan interface chosen as underlay must have vlanconfig spanning all nodes
	if err := v.checkVCSpansAllNodes(newhnc.Spec.ClusterNetwork); err != nil {
		return fmt.Errorf(updateErr, newhnc.Name, err)
	}

	matchedNode, err := v.nodeSelectorMatchesAllNodes(newhnc.Spec.NodeSelector)
	if err != nil {
		return fmt.Errorf(updateErr, newhnc.Name, err)
	}
	if !matchedNode {
		return fmt.Errorf(updateErr, newhnc.Name, fmt.Errorf("node selector does not match all nodes"))
	}

	return nil
}

func (v *Validator) Delete(_ *admission.Request, oldObj runtime.Object) error {
	hnc, ok := oldObj.(*networkv1.HostNetworkConfig)
	if !ok || hnc == nil {
		return nil
	}

	//since ovn is already using the interface as underlay, user should disable it first in hostnetworkconfig before deleting it.
	if hnc.Spec.Underlay {
		return fmt.Errorf(deleteErr, hnc.Name, fmt.Errorf("underlay is enabled, disable underlay first"))
	}

	return nil
}

func (v *Validator) checkVM(nad *nadv1.NetworkAttachmentDefinition) error {
	vmGetter := utils.NewVMGetter(v.vmCache)

	if vmStrList, err := vmGetter.VMNamesWhoUseNad(nad); err != nil {
		return err
	} else if len(vmStrList) > 0 {
		return fmt.Errorf("it's still used by VM(s) %s which must remove the related networks and interfaces", strings.Join(vmStrList, ", "))
	}

	return nil
}

// check if any vm exists for overlay nads (irrespective of clusternetwork)
func (v *Validator) checkifVMExistsForOverlayNADs() (err error) {
	// check if nad exists
	var nadList []*nadv1.NetworkAttachmentDefinition
	nadGetter := utils.NewNadGetter(v.nadCache)
	if nadList, err = nadGetter.ListAllNads(); err != nil {
		return fmt.Errorf("nad does not exist err %w", err)
	}

	for _, nad := range nadList {
		if utils.IsOverlayNad(nad) {
			if err := v.checkVM(nad); err != nil {
				return err
			}
		}
	}

	return nil
}

// sameSubnet validates CIDR formats, enforces family conformance, and ensures all entries belong to the same subnet.
func sameSubnet(cidrs []string, familyName string) error {
	if len(cidrs) == 0 {
		return nil
	}

	isIPv4 := familyName == "IPv4"

	// Helper closure to parse, format-check, and enforce IP family conformance
	parseAndValidate := func(cidr string) (netip.Prefix, error) {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("invalid %s CIDR format %q: %w", familyName, cidr, err)
		}
		if isIPv4 && !prefix.Addr().Is4() {
			return netip.Prefix{}, fmt.Errorf("expected IPv4 address for %s, got %q", familyName, cidr)
		}
		if !isIPv4 && !prefix.Addr().Is6() {
			return netip.Prefix{}, fmt.Errorf("expected IPv6 address for %s, got %q", familyName, cidr)
		}
		return prefix, nil
	}

	// Always parse and validate the first CIDR
	refPrefix, err := parseAndValidate(cidrs[0])
	if err != nil {
		return err
	}

	if len(cidrs) == 1 {
		return nil
	}

	// The reset should be in same subnet
	refSubnet := refPrefix.Masked()
	for i := 1; i < len(cidrs); i++ {
		prefix, err := parseAndValidate(cidrs[i])
		if err != nil {
			return err
		}
		if prefix.Bits() != refPrefix.Bits() || prefix.Masked().Addr() != refSubnet.Addr() {
			return fmt.Errorf("static %ss are not in the same subnet", familyName)
		}
	}
	return nil
}

func validateStaticIPsForNodes(hnc *networkv1.HostNetworkConfig, nodes []*v1.Node) error {
	// Refuse HNCs with completely empty IP maps upfront. While a node selector
	// might match 0 nodes (e.g., if the object serves as an unassigned placeholder or template),
	// having zero static IP configurations in static mode is strictly a user mistake
	// that would otherwise create unnecessary reconciliation burdens for the controller.
	if len(hnc.Spec.HostIPs) == 0 && len(hnc.Spec.HostMultiFamilyIPs) == 0 {
		return fmt.Errorf("at least one static IP configuration must be provided")
	}

	ipFamily := hnc.Spec.IPFamily
	if ipFamily == "" {
		ipFamily = networkv1.IPFamilyIPv4 // Default fallback
	}

	legacyMode := len(hnc.Spec.HostIPs) > 0

	// 1. Ensure every matched node has an entry in the map (map cannot have fewer entries than nodes,
	// but it is allowed to contain extra placeholder/leftover records).
	for _, node := range nodes {
		if legacyMode {
			if _, ok := hnc.Spec.HostIPs[node.Name]; !ok {
				return fmt.Errorf("static IP not found for node %s", node.Name)
			}
			continue
		}

		mfConfig, ok := hnc.Spec.HostMultiFamilyIPs[node.Name]
		if !ok {
			return fmt.Errorf("static IP configuration not found for node %s", node.Name)
		}

		switch ipFamily {
		case networkv1.IPFamilyIPv4:
			if mfConfig.IPv4 == nil {
				return fmt.Errorf("IPv4 address is required for node %s under ipv4 family", node.Name)
			}
		case networkv1.IPFamilyIPv6:
			if mfConfig.IPv6 == nil {
				return fmt.Errorf("IPv6 address is required for node %s under ipv6 family", node.Name)
			}
		case networkv1.IPFamilyDualIPv4First:
			if mfConfig.IPv4 == nil || mfConfig.IPv6 == nil {
				return fmt.Errorf("both IPv4 and IPv6 addresses are required for node %s under dual-stack family", node.Name)
			}
		default:
			return fmt.Errorf("unsupported IP family: %s", ipFamily)
		}
	}

	// 2. Collect ALL entries from the map for validation
	ipv4CIDRs, ipv6CIDRs := collectAllIPCIDRs(hnc)

	// 3. Enforce strict family conformance on all collected map entries
	switch ipFamily {
	case networkv1.IPFamilyIPv4:
		if len(ipv6CIDRs) > 0 {
			return fmt.Errorf("IPv6 addresses are not allowed when IP family is IPv4")
		}
	case networkv1.IPFamilyIPv6:
		if len(ipv4CIDRs) > 0 {
			return fmt.Errorf("IPv4 addresses are not allowed when IP family is IPv6")
		}
	case networkv1.IPFamilyDualIPv4First:
		if len(ipv4CIDRs) != len(ipv6CIDRs) {
			return fmt.Errorf("dual-stack static IPs count mismatch: IPv4 count (%d) and IPv6 count (%d) must match", len(ipv4CIDRs), len(ipv6CIDRs))
		}
	}

	// 4. Validate subnet consistency and CIDR formatting independently per family.
	// No matter how many records are present (including placeholders/leftovers),
	// all of them must strictly follow the same subnet rule.
	if err := sameSubnet(ipv4CIDRs, "IPv4"); err != nil {
		return err
	}
	if err := sameSubnet(ipv6CIDRs, "IPv6"); err != nil {
		return err
	}

	// 5. Validate that no two nodes are assigned the exact same static IP address.
	if err := validateUniqueIPs(ipv4CIDRs, "IPv4"); err != nil {
		return err
	}
	if err := validateUniqueIPs(ipv6CIDRs, "IPv6"); err != nil {
		return err
	}

	return nil
}

func (v *Validator) validateStaticIPs(hnc *networkv1.HostNetworkConfig) error {
	// 1. Check ip faminly and ip map
	if err := validateIPFamilyAndIP(hnc); err != nil {
		return err
	}

	// 2. Resolve label selector into a usable selector
	var selector labels.Selector
	if hnc.Spec.NodeSelector != nil {
		var err error
		selector, err = metav1.LabelSelectorAsSelector(hnc.Spec.NodeSelector)
		if err != nil {
			return fmt.Errorf("failed to parse node selector: %w", err)
		}
	} else {
		selector = labels.Everything()
	}

	// 3. Fetch only the nodes matching the selector directly from the cache
	nodes, err := v.nodeCache.List(selector)
	if err != nil {
		return fmt.Errorf("failed to list nodes matching selector: %w", err)
	}

	// 3. Run validation on the filtered node slice
	return validateStaticIPsForNodes(hnc, nodes)
}

func (v *Validator) validateHostNetworkConfig(newhnc *networkv1.HostNetworkConfig, op ValidateOp) (err error) {
	hostnetworkconfigs, err := v.hncCache.List(labels.Everything())
	if err != nil {
		return err
	}

	for _, hostnetworkconfig := range hostnetworkconfigs {
		if hostnetworkconfig.DeletionTimestamp != nil {
			continue
		}

		// Skip comparing the CR against itself during updates
		if hostnetworkconfig.Name == newhnc.Name {
			continue
		}

		//no more than one underlay exists for overlay networks. If user has to change underlay setting, old underlay has to be disabled first.
		if hostnetworkconfig.Spec.Underlay && newhnc.Spec.Underlay {
			return fmt.Errorf("hostnetworkconfig %s already exists for cluster network %s vlanid %d with underlay enabled", hostnetworkconfig.Name, newhnc.Spec.ClusterNetwork, hostnetworkconfig.Spec.VlanID)
		}

		if hostnetworkconfig.Spec.ClusterNetwork != newhnc.Spec.ClusterNetwork {
			continue
		}

		// Enforce VLAN ID uniqueness on creation only to avoid blocking updates on existing duplicate states
		if op == ValidateCreate && hostnetworkconfig.Spec.VlanID == newhnc.Spec.VlanID {
			return fmt.Errorf("hostnetworkconfig %s already exists for cluster network %s with vlanID %d",
				hostnetworkconfig.Name, hostnetworkconfig.Spec.ClusterNetwork, hostnetworkconfig.Spec.VlanID)
		}
	}

	if newhnc.Spec.Mode == IPModeStatic {
		if err := v.validateStaticIPs(newhnc); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) checkVlanStatusReady(clusterNetwork string) error {
	//mgmt cluster
	if clusterNetwork == utils.ManagementClusterNetworkName {
		return nil
	}

	_, err := v.cnCache.Get(clusterNetwork)
	if err != nil {
		return fmt.Errorf("cluster network %s not found because %v", clusterNetwork, err)
	}

	vsList, err := v.vsCache.List(labels.Set{
		utils.KeyClusterNetworkLabel: clusterNetwork,
	}.AsSelector())
	if err != nil {
		return err
	}

	if len(vsList) == 0 {
		return fmt.Errorf("vlan status not present for cluster network %s", clusterNetwork)
	}

	for _, vs := range vsList {
		if networkv1.Ready.IsFalse(vs.Status) {
			return fmt.Errorf("vs %s status is not Ready", vs.Name)
		}
	}

	return nil
}

func getMatchNodes(vc *networkv1.VlanConfig) ([]string, error) {
	var matchedNodes []string
	//skip not ready vlan configs because they will not be applied to any nodes, and if there is at least one ready vlan config spanning all nodes, the network will work fine
	//There will be atleast 1 vlanconfig with non-empty matched nodes, else it will be caught by checkVlanStatusReady (len(vsList) will be 0 if there is no vlanconfig)
	if vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" || vc.Annotations[utils.KeyMatchedNodes] == "[]" {
		return matchedNodes, nil
	}

	if err := json.Unmarshal([]byte(vc.Annotations[utils.KeyMatchedNodes]), &matchedNodes); err != nil {
		return nil, err
	}

	return matchedNodes, nil
}

func matchNode(node *v1.Node, selector labels.Selector) (bool, error) {
	if node == nil {
		return false, fmt.Errorf("node not found")
	}

	if node.DeletionTimestamp != nil {
		return false, nil
	}

	if !selector.Matches(labels.Set(node.Labels)) {
		// node doesn't match the selector, skip processing
		return false, nil
	}

	return true, nil
}

func (v *Validator) nodeSelectorMatchesAllNodes(nodeSelector *metav1.LabelSelector) (bool, error) {
	if nodeSelector == nil {
		// if node selector is nil, all nodes match by default, return true
		return true, nil
	}

	selector, err := metav1.LabelSelectorAsSelector(nodeSelector)
	if err != nil {
		return false, err
	}

	nodes, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return false, err
	}

	//check if nodeselector matches all nodes in the cluster if the underlay is enabled
	for _, node := range nodes {
		//skip witness node as vlanconfig does not span witness node, HostNetworkConfig too
		if utils.HasWitnessNodeLabelKey(node.Labels) {
			continue
		}

		matchNode, err := matchNode(node, selector)
		if err != nil {
			return false, fmt.Errorf("error matching node %s with node selector: %w", node.Name, err)
		}
		if !matchNode {
			return false, nil
		}
	}

	return true, nil
}

func (v *Validator) checkVCSpansAllNodes(clusterNetwork string) error {
	//mgmt cluster
	if clusterNetwork == utils.ManagementClusterNetworkName {
		return nil
	}

	matchedNodes := mapset.NewSet[string]()

	vcs, err := v.vcCache.List(labels.Set{
		utils.KeyClusterNetworkLabel: clusterNetwork,
	}.AsSelector())
	if err != nil {
		return err
	}

	if len(vcs) == 0 {
		return fmt.Errorf("vlan config not present for cluster network %s", clusterNetwork)
	}

	for _, vc := range vcs {
		vnodes, err := getMatchNodes(vc)
		if err != nil {
			return err
		}
		matchedNodes.Append(vnodes...)
	}

	nodes, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return err
	}

	//check if vlanconfig contains all the nodes in the cluster
	for _, node := range nodes {
		if node.DeletionTimestamp != nil {
			continue
		}

		//skip witness node as vlanconfig does not span witness node
		if utils.HasWitnessNodeLabelKey(node.Labels) {
			continue
		}

		if !matchedNodes.Contains(node.Name) {
			return fmt.Errorf("vlanconfig does not span %s", node.Name)
		}
	}

	return nil
}

func (v *Validator) Resource() admission.Resource {
	return admission.Resource{
		Names:      []string{"hostnetworkconfigs"},
		Scope:      admissionregv1.ClusterScope,
		APIGroup:   networkv1.SchemeGroupVersion.Group,
		APIVersion: networkv1.SchemeGroupVersion.Version,
		ObjectType: &networkv1.HostNetworkConfig{},
		OperationTypes: []admissionregv1.OperationType{
			admissionregv1.Create,
			admissionregv1.Update,
			admissionregv1.Delete,
		},
	}
}

// High-level IP family vs. IP map fields validation
// Detailed IPs are checked on validateStaticIPs
func validateIPFamilyAndIP(hnc *networkv1.HostNetworkConfig) error {
	hasLegacyHostIPs := len(hnc.Spec.HostIPs) > 0
	hasMultiFamilyIPs := len(hnc.Spec.HostMultiFamilyIPs) > 0

	// HostIPs and HostMultiFamilyIPs are strictly mutually exclusive
	if hasLegacyHostIPs && hasMultiFamilyIPs {
		return fmt.Errorf("cannot specify both HostIPs and HostMultiFamilyIPs")
	}

	ipFamily := hnc.Spec.IPFamily
	if ipFamily == "" {
		ipFamily = networkv1.IPFamilyIPv4 // Default fallback
	}

	switch ipFamily {
	case networkv1.IPFamilyIPv4:
		// IPv4-only family permits either legacy HostIPs or HostMultiFamilyIPs
	case networkv1.IPFamilyIPv6, networkv1.IPFamilyDualIPv4First:
		if hasLegacyHostIPs {
			return fmt.Errorf("legacy HostIPs are not supported under %v family; use HostMultiFamilyIPs", ipFamily)
		}
		if !hasMultiFamilyIPs {
			return fmt.Errorf("HostMultiFamilyIPs is required under %v family", ipFamily)
		}
	default:
		return fmt.Errorf("unsupported IP family: %s", ipFamily)
	}

	return nil
}

// collectAllIPCIDRs extracts and returns all IPv4 and IPv6 CIDR strings from either HostIPs
// or HostMultiFamilyIPs. Assumes input structure and IP families have already been validated.
func collectAllIPCIDRs(hnc *networkv1.HostNetworkConfig) (ipv4CIDRs []string, ipv6CIDRs []string) {
	if len(hnc.Spec.HostIPs) > 0 {
		ipv4CIDRs = make([]string, 0, len(hnc.Spec.HostIPs))
		for _, ip := range hnc.Spec.HostIPs {
			ipv4CIDRs = append(ipv4CIDRs, string(ip))
		}
		return ipv4CIDRs, nil
	}

	// MultiFamilyMode
	ipv4CIDRs = make([]string, 0, len(hnc.Spec.HostMultiFamilyIPs))
	ipv6CIDRs = make([]string, 0, len(hnc.Spec.HostMultiFamilyIPs))

	for _, mfConfig := range hnc.Spec.HostMultiFamilyIPs {
		if mfConfig.IPv4 != nil {
			ipv4CIDRs = append(ipv4CIDRs, string(*mfConfig.IPv4))
		}
		if mfConfig.IPv6 != nil {
			ipv6CIDRs = append(ipv6CIDRs, string(*mfConfig.IPv6))
		}
	}
	return ipv4CIDRs, ipv6CIDRs
}

// validateUniqueIPs ensures no duplicate static IP addresses are assigned.
func validateUniqueIPs(cidrs []string, familyName string) error {
	if len(cidrs) <= 1 {
		return nil
	}

	seen := make(map[string]struct{}, len(cidrs))
	expectedLen := 0

	// Duplications happen rarely, so we avoid checking map existence on every iteration
	// (which would add extra lookups in the common path). Instead, we unconditionally
	// insert and increment our counter. Since Go's map len operation is O(1) with
	// no lookup overhead, comparing sizes is extremely fast.
	for _, cidr := range cidrs {
		seen[cidr] = struct{}{}
		expectedLen++
		if len(seen) < expectedLen {
			return fmt.Errorf("duplicate static %s %q is assigned to multiple nodes", familyName, cidr)
		}
	}
	return nil
}
