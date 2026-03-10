package hostnetworkconfig

import (
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/harvester/webhook/pkg/server/admission"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	networkv1 "github.com/harvester/harvester-network-controller/pkg/apis/network.harvesterhci.io/v1beta1"
	ctlcniv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/k8s.cni.cncf.io/v1"
	ctlkubevirtv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/kubevirt.io/v1"
	ctlnetworkv1 "github.com/harvester/harvester-network-controller/pkg/generated/controllers/network.harvesterhci.io/v1beta1"
	"github.com/harvester/harvester-network-controller/pkg/utils"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	ctlcorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	hnc := oldObj.(*networkv1.HostNetworkConfig)

	if !hnc.Spec.Underlay {
		//delete hostnetworkconfig if its not serving as underlay
		return nil
	}

	if err := v.checkifVMExistsForOverlayNADs(); err != nil {
		return fmt.Errorf(deleteErr, hnc.Name, err)
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

func sameSubnet(cidrs []string) (bool, error) {
	if len(cidrs) == 0 {
		return true, nil
	}
	var refMaskLength int

	// Parse the first CIDR as the reference subnet
	_, refNet, err := net.ParseCIDR(cidrs[0])
	refMaskLength, _ = refNet.Mask.Size()
	if err != nil {
		return false, err
	}

	for _, c := range cidrs[1:] {
		_, net1, err := net.ParseCIDR(c)
		if err != nil {
			return false, err
		}
		m2, _ := net1.Mask.Size()

		if refMaskLength != m2 || !refNet.IP.Equal(net1.IP) {
			return false, nil
		}
	}

	return true, nil
}

func validateStaticIPsForAllNodes(nodes []*v1.Node, hostIPs map[string]networkv1.IPAddr) error {
	cidrs := make([]string, 0, len(hostIPs))
	for _, node := range nodes {
		if _, ok := hostIPs[node.Name]; !ok {
			return fmt.Errorf("static IP not found for node %s", node.Name)
		}
		cidrs = append(cidrs, string(hostIPs[node.Name]))
	}

	//check if all static IPs are in the same subnet
	if ok, err := sameSubnet(cidrs); !ok {
		return fmt.Errorf("static IPs are not in the same subnet: %v", err)
	}

	return nil
}

func validateStaticIPsForNodeWithLabel(nodes []*v1.Node, hostIPs map[string]networkv1.IPAddr, selector labels.Selector) error {
	cidrs := make([]string, 0, len(hostIPs))
	for _, node := range nodes {
		matched, err := matchNode(node, selector)
		if err != nil {
			return fmt.Errorf("error matching node %s with node selector: %w", node.Name, err)
		}
		if matched {
			_, ok := hostIPs[node.Name]
			if !ok {
				return fmt.Errorf("static IP not found for node %s", node.Name)
			}
			cidrs = append(cidrs, string(hostIPs[node.Name]))
		}
	}

	//check if all static IPs are in the same subnet
	if ok, err := sameSubnet(cidrs); !ok {
		return fmt.Errorf("static IPs are not in the same subnet: %v", err)
	}

	return nil
}

func (v *Validator) validateStaticIPs(hostIPs map[string]networkv1.IPAddr, nodeSelector *metav1.LabelSelector) error {
	nodes, err := v.nodeCache.List(labels.Everything())
	if err != nil {
		return err
	}

	if nodeSelector == nil {
		return validateStaticIPsForAllNodes(nodes, hostIPs)
	} else {
		selector, err := metav1.LabelSelectorAsSelector(nodeSelector)
		if err != nil {
			return err
		}
		return validateStaticIPsForNodeWithLabel(nodes, hostIPs, selector)
	}
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

		//no more than one underlay exists for overlay networks. If user has to change underlay setting, old underlay has to be disabled first.
		if hostnetworkconfig.Spec.Underlay && newhnc.Spec.Underlay {
			return fmt.Errorf("hostnetworkconfig %s already exists for cluster network %s vlanid %d with underlay enabled", hostnetworkconfig.Name, newhnc.Spec.ClusterNetwork, hostnetworkconfig.Spec.VlanID)
		}

		if hostnetworkconfig.Spec.ClusterNetwork != newhnc.Spec.ClusterNetwork {
			continue
		}

		if op == ValidateCreate && hostnetworkconfig.Spec.VlanID == newhnc.Spec.VlanID {
			return fmt.Errorf("hostnetworkconfig %s already exists for cluster network %s with vlanID %d", hostnetworkconfig.Name, hostnetworkconfig.Spec.ClusterNetwork, hostnetworkconfig.Spec.VlanID)
		}
	}

	if newhnc.Spec.Mode == IPModeStatic {
		if err := v.validateStaticIPs(newhnc.Spec.HostIPs, newhnc.Spec.NodeSelector); err != nil {
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
	if vc.Annotations == nil || vc.Annotations[utils.KeyMatchedNodes] == "" {
		return nil, fmt.Errorf("vlan config annotations is absent for matched nodes")
	}

	var matchedNodes []string
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
