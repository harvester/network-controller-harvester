package v1beta1

import (
	"net"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vc;vcs,scope=Cluster
// +kubebuilder:printcolumn:name="CLUSTERNETWORK",type=string,JSONPath=`.spec.clusterNetwork`
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=`.metadata.creationTimestamp`

type VlanConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VlanConfigSpec `json:"spec"`
}

type VlanConfigSpec struct {
	// +optional
	Description    string            `json:"description,omitempty"`
	ClusterNetwork string            `json:"clusterNetwork"`
	NodeSelector   map[string]string `json:"nodeSelector,omitempty"`
	Uplink         Uplink            `json:"uplink"`
}

type Uplink struct {
	NICs []string `json:"nics,omitempty"`
	// +optional
	LinkAttrs *LinkAttrs `json:"linkAttributes,omitempty"`
	// +optional
	BondOptions *BondOptions `json:"bondOptions,omitempty"`
}

type LinkAttrs struct {
	// +optional
	// +kubebuilder:validation:Minimum:=0
	MTU int `json:"mtu,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum:=-1
	// +kubebuilder:default:=-1
	TxQLen int `json:"txQLen,omitempty"`
	// +optional
	HardwareAddr net.HardwareAddr `json:"hardwareAddr,omitempty"`
}

// reference: https://www.kernel.org/doc/Documentation/networking/bonding.txt
type BondOptions struct {
	// +optional
	// +kubebuilder:default:="active-backup"
	Mode BondMode `json:"mode,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum:=-1
	Miimon int `json:"miimon,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"layer2","layer2+3","layer3+4","encap2+3","encap3+4"}
	// +kubebuilder:default:="layer2"
	XmitHashPolicy string `json:"xmitHashPolicy,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"slow","fast"}
	LacpRate string `json:"lacpRate,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"stable","bandwidth","count"}
	AdSelect string `json:"adSelect,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum:=-1
	// +kubebuilder:default:=-1
	ArpInterval int `json:"arpInterval,omitempty"`
	// +optional
	// +kubebuilder:validation:MaxItems:=16
	// +kubebuilder:validation:Item:Pattern:="^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$"
	ArpIpTargets []string `json:"arpIpTargets,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"none","active","backup","all","filter", "filter_active", "filter_backup"}
	// +kubebuilder:default:="none"
	ArpValidate string `json:"arpValidate,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"any", "all"}
	// +kubebuilder:default:="any"
	ArpAllTargets string `json:"arpAllTargets,omitempty"`
}

// +kubebuilder:validation:Enum={"balance-rr","active-backup","balance-xor","broadcast","802.3ad","balance-tlb","balance-alb"}

type BondMode string

const (
	BondModeBalanceRr    BondMode = "balance-rr"
	BondMoDeActiveBackup BondMode = "active-backup"
	BondModeBalanceXor   BondMode = "balance-xor"
	BondModeBroadcast    BondMode = "broadcast"
	BondMode8023AD       BondMode = "802.3ad"
	BondModeBalanceTlb   BondMode = "balance-tlb"
	BondModeBalanceAlb   BondMode = "balance-alb"
)
