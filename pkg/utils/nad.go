package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	ctlcniv1 "github.com/harvester/harvester/pkg/generated/controllers/k8s.cni.cncf.io/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	cniv1 "github.com/containernetworking/cni/pkg/types"
	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	harvesterutil "github.com/harvester/harvester/pkg/util"
)

const (
	CNITypeKubeOVN = "kube-ovn"
	ovnProvider    = "ovn"

	CNITypeBridge       = "bridge"
	CNITypeDefaultEmpty = "" // potential empty type, is treated as CNITypeBridge
)

type Connectivity string

const (
	Connectable   Connectivity = "true"
	Unconnectable Connectivity = "false"
	DHCPFailed    Connectivity = "DHCP failed"
	PingFailed    Connectivity = "ping failed"
)

type Mode string

const (
	Auto   Mode = "auto"
	Manual Mode = "manual"
)

type NetworkType string

const (
	L2VlanNetwork   NetworkType = "L2VlanNetwork"
	UntaggedNetwork NetworkType = "UntaggedNetwork"
	OverlayNetwork  NetworkType = "OverlayNetwork"
)

type NadSelectedNetworks []nadv1.NetworkSelectionElement

type Layer3NetworkConf struct {
	Mode         Mode         `json:"mode,omitempty"`
	CIDR         string       `json:"cidr,omitempty"`
	Gateway      string       `json:"gateway,omitempty"`
	ServerIPAddr string       `json:"serverIPAddr,omitempty"`
	Connectivity Connectivity `json:"connectivity,omitempty"`
	Outdated     bool         `json:"outdated,omitempty"`
}

func NewLayer3NetworkConf(conf string) (*Layer3NetworkConf, error) {
	if conf == "" {
		return &Layer3NetworkConf{}, nil
	}

	networkConf := &Layer3NetworkConf{}

	if err := json.Unmarshal([]byte(conf), networkConf); err != nil {
		return nil, fmt.Errorf("unmarshal %s faield, error: %w", conf, err)
	}

	// validate
	if networkConf.Mode != "" && networkConf.Mode != Auto && networkConf.Mode != Manual {
		return nil, fmt.Errorf("unknown mode %s", networkConf.Mode)
	}

	// validate cidr and gateway when the mode is manual
	if networkConf.Mode == Manual {
		_, ipnet, err := net.ParseCIDR(networkConf.CIDR)
		if err != nil || (ipnet != nil && isMaskZero(ipnet)) {
			return nil, fmt.Errorf("the CIDR %s is invalid", networkConf.CIDR)
		}

		if net.ParseIP(networkConf.Gateway) == nil {
			return nil, fmt.Errorf("the gateway %s is invalid", networkConf.Gateway)
		}
	}

	return networkConf, nil
}

func (c *Layer3NetworkConf) ToString() (string, error) {
	bytes, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func NewNADSelectedNetworks(conf string) (NadSelectedNetworks, error) {
	networks := make([]nadv1.NetworkSelectionElement, 1)
	if err := json.Unmarshal([]byte(conf), &networks); err != nil {
		return nil, err
	}

	return networks, nil
}

func (n NadSelectedNetworks) ToString() (string, error) {
	bytes, err := json.Marshal(n)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

// L2 mode
type NetConf struct {
	cniv1.NetConf
	BrName       string       `json:"bridge"`
	IsGW         bool         `json:"isGateway"`
	IsDefaultGW  bool         `json:"isDefaultGateway"`
	ForceAddress bool         `json:"forceAddress"`
	IPMasq       bool         `json:"ipMasq"`
	MTU          int          `json:"mtu"`
	HairpinMode  bool         `json:"hairpinMode"`
	PromiscMode  bool         `json:"promiscMode"`
	Vlan         int          `json:"vlan"`
	Provider     string       `json:"provider"`
	VlanTrunk    []*VlanTrunk `json:"vlanTrunk,omitempty"`
}

type VlanTrunk struct {
	MinID *int `json:"minID,omitempty"`
	MaxID *int `json:"maxID,omitempty"`
	ID    *int `json:"id,omitempty"`
}

func (item *VlanTrunk) IsValid() (bool, error) {
	if item == nil {
		return true, nil
	}

	switch {
	case item.MinID != nil && item.MaxID != nil:
		minID := *item.MinID
		if minID < MinTrunkVlanID || minID > MaxVlanID {
			return false, fmt.Errorf("incorrect trunk minID parameter %v", minID)
		}
		maxID := *item.MaxID
		if maxID < MinTrunkVlanID || maxID > MaxVlanID {
			return false, fmt.Errorf("incorrect trunk maxID parameter %v", maxID)
		}
		if maxID < minID {
			return false, fmt.Errorf("minID %v is greater than maxID %v in trunk parameter", minID, maxID)
		}
	case item.MinID == nil && item.MaxID != nil:
		return false, errors.New("minID and maxID should be configured simultaneously, minID is missing")
	case item.MinID != nil && item.MaxID == nil:
		return false, errors.New("minID and maxID should be configured simultaneously, maxID is missing")
	}

	// single vid
	if item.ID != nil {
		ID := *item.ID
		if ID < MinTrunkVlanID || ID > MaxVlanID {
			return false, fmt.Errorf("incorrect trunk id parameter %v", ID)
		}
	}

	return true, nil
}

// if related vlanconfig is valid
func (nc *NetConf) IsVlanConfigValid() (bool, error) {
	if nc.Vlan < MinVlanID || nc.Vlan > MaxVlanID {
		return false, fmt.Errorf("vlan %v is out of range [%v .. %v]", nc.Vlan, MinVlanID, MaxVlanID)
	}

	for _, vt := range nc.VlanTrunk {
		if _, err := vt.IsValid(); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (nc *NetConf) dumpVlanIDSet() (*VlanIDSet, error) {
	vis := NewVlanIDSet()
	var err error
	if err = vis.SetVID(nc.Vlan); err != nil {
		return nil, err
	}

	for _, vt := range nc.VlanTrunk {
		if vt.ID != nil {
			if err = vis.SetVID(*vt.ID); err != nil {
				return nil, err
			}
		}
		if vt.MinID != nil && vt.MaxID != nil {
			for i := *vt.MinID; i <= *vt.MaxID; i++ {
				if err = vis.SetVID(i); err != nil {
					return nil, err
				}
			}
		}
	}
	return vis, nil
}

func NewVlanIDSetFromNetConf(nc *NetConf) (*VlanIDSet, error) {
	if nc == nil {
		return nil, fmt.Errorf("the input netconf is empty")
	}
	if !nc.IsBridgeCNI() {
		return nil, fmt.Errorf("the netconf is not bridge type")
	}

	if _, err := nc.IsVlanConfigValid(); err != nil {
		return nil, err
	}

	return nc.dumpVlanIDSet()
}

// return vidsets from all bridge nads
func NewVlanIDSetFromNadList(nads []*nadv1.NetworkAttachmentDefinition) (*VlanIDSet, error) {
	vis := NewVlanIDSet()
	if len(nads) == 0 {
		return vis, nil
	}

	for _, nad := range nads {
		if nad.DeletionTimestamp != nil {
			continue
		}
		nc, err := DecodeNadConfigToNetConf(nad)
		if err != nil {
			return nil, err
		}

		// skip non-bridge CNI
		if !nc.IsBridgeCNI() {
			continue
		}

		tmpvis, err := NewVlanIDSetFromNetConf(nc)
		if err != nil {
			return nil, err
		}
		vis = vis.Append(tmpvis)
	}

	return vis, nil
}

// return vidsets from all vlan trunk mode bridge nads
func NewVlanIDSetFromTrunkModeNadList(nads []*nadv1.NetworkAttachmentDefinition) (*VlanIDSet, error) {
	vis := NewVlanIDSet()
	if len(nads) == 0 {
		return vis, nil
	}

	for _, nad := range nads {
		if nad.DeletionTimestamp != nil {
			continue
		}
		nc, err := DecodeNadConfigToNetConf(nad)
		if err != nil {
			return nil, err
		}

		// skip non-bridge CNI
		if !nc.IsBridgeCNI() || !nc.IsVlanTrunkMode() {
			continue
		}

		tmpvis, err := NewVlanIDSetFromNetConf(nc)
		if err != nil {
			return nil, err
		}
		vis = vis.Append(tmpvis)
	}

	return vis, nil
}

// if VlanTrunk is configured
func (nc *NetConf) IsVlanTrunkMode() bool {
	return len(nc.VlanTrunk) > 0
}

// the default mode
func (nc *NetConf) IsVlanAccessMode() bool {
	return len(nc.VlanTrunk) == 0
}

func (nc *NetConf) IsBridgeCNI() bool {
	return nc.Type == CNITypeBridge || nc.Type == CNITypeDefaultEmpty
}

func (nc *NetConf) IsKubeOVNCNI() bool {
	return nc.Type == CNITypeKubeOVN
}

func IsVlanNad(nad *nadv1.NetworkAttachmentDefinition) bool {
	if nad == nil || nad.Spec.Config == "" || nad.Labels == nil || nad.Labels[KeyNetworkType] == "" ||
		nad.Labels[KeyClusterNetworkLabel] == "" || nad.Labels[KeyVlanLabel] == "" {
		return false
	}

	return true
}

// decode nad config string to a config struct
func DecodeNadConfigToNetConf(nad *nadv1.NetworkAttachmentDefinition) (*NetConf, error) {
	conf := &NetConf{}
	if nad.Spec.Config == "" {
		return conf, nil
	}

	if err := json.Unmarshal([]byte(nad.Spec.Config), &conf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal nad %v/%v config %s %w", nad.Namespace, nad.Name, nad.Spec.Config, err)
	}

	return conf, nil
}

func isMaskZero(ipnet *net.IPNet) bool {
	for _, b := range ipnet.Mask {
		if b != 0 {
			return false
		}
	}

	return true
}

// if this nad is a storagenetwork nad
func IsStorageNetworkNad(nad *nadv1.NetworkAttachmentDefinition) bool {
	if nad == nil || nad.Namespace != harvesterutil.HarvesterSystemNamespaceName {
		return false
	}

	// seems Harvester webhook has no protection on this annotation
	if nad.Annotations != nil && nad.Annotations[StorageNetworkAnnotation] == "true" {
		return true
	}

	// check name
	if strings.HasPrefix(nad.Name, StorageNetworkNetAttachDefPrefix) {
		return true
	}

	return false
}

// filter the first active storage network nad from a list of nads
func FilterFirstActiveStorageNetworkNad(nads []*nadv1.NetworkAttachmentDefinition) *nadv1.NetworkAttachmentDefinition {
	if len(nads) == 0 {
		return nil
	}
	for _, nad := range nads {
		if IsStorageNetworkNad(nad) && nad.DeletionTimestamp == nil {
			return nad
		}
	}
	return nil
}

type NadGetter struct {
	nadCache ctlcniv1.NetworkAttachmentDefinitionCache
}

func NewNadGetter(nadCache ctlcniv1.NetworkAttachmentDefinitionCache) *NadGetter {
	return &NadGetter{nadCache: nadCache}
}

// list all nads attached to a cluster network
func (n *NadGetter) ListNadsOnClusterNetwork(cnName string) ([]*nadv1.NetworkAttachmentDefinition, error) {
	nads, err := n.nadCache.List(corev1.NamespaceAll, labels.Set(map[string]string{
		KeyClusterNetworkLabel: cnName,
	}).AsSelector())
	if err != nil {
		return nil, err
	}

	if len(nads) == 0 {
		return nil, nil
	}
	return nads, nil
}

func (n *NadGetter) GetFirstActiveStorageNetworkNadOnClusterNetwork(cnName string) (*nadv1.NetworkAttachmentDefinition, error) {
	nads, err := n.nadCache.List(harvesterutil.HarvesterSystemNamespaceName, labels.Set(map[string]string{
		KeyClusterNetworkLabel: cnName,
	}).AsSelector())
	if err != nil {
		return nil, err
	}

	if len(nads) == 0 {
		return nil, nil
	}

	return FilterFirstActiveStorageNetworkNad(nads), nil
}

func (n *NadGetter) NadNamesOnClusterNetwork(cnName string) ([]string, error) {
	nads, err := n.ListNadsOnClusterNetwork(cnName)
	if err != nil {
		return nil, err
	}
	return generateNadNameList(nads), nil
}

func generateNadNameList(nads []*nadv1.NetworkAttachmentDefinition) []string {
	if len(nads) == 0 {
		return nil
	}
	nadStrList := make([]string, len(nads))
	for i, nad := range nads {
		nadStrList[i] = nad.Namespace + "/" + nad.Name
	}
	return nadStrList
}

func GetNadNameFromProvider(provider string) (nadName string, nadNamespace string, err error) {
	if provider == "" {
		return "", "", fmt.Errorf("provider is empty for cni type %s", CNITypeKubeOVN)
	}

	nad := strings.Split(provider, ".")
	if len(nad) < 3 {
		return "", "", fmt.Errorf("invalid provider length %d for provider %s", len(nad), provider)
	}

	return nad[0], nad[1], nil
}

func GetProviderFromNad(nadName string, nadNamespace string) (provider string, err error) {
	if nadName == "" {
		return "", fmt.Errorf("nad name %s is empty", nadName)
	}

	if nadNamespace == "" {
		nadNamespace = defaultNamespace
	}

	return nadName + "." + nadNamespace + "." + ovnProvider, nil
}
