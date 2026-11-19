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
	// Mode selects how the NICs are attached to the cluster network bridge.
	// "" or "bond" (default): the NICs are enslaved to the bond <cn>-bo, which is the only bridge port.
	// "isolated": every NIC is attached to the bridge directly as an isolated bridge port and no bond is created.
	// The bridge never forwards between isolated ports, which suits a switchless full mesh where each NIC is
	// cabled to a different peer. BondOptions are ignored in this mode.
	// +optional
	Mode UplinkMode `json:"mode,omitempty"`
}

// +kubebuilder:validation:Enum={"","bond","isolated"}

type UplinkMode string

const (
	UplinkModeBond     UplinkMode = "bond"
	UplinkModeIsolated UplinkMode = "isolated"
)

func (u *Uplink) IsIsolated() bool {
	return u.Mode == UplinkModeIsolated
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
	// +kubebuilder:default:=-1
	Miimon int `json:"miimon,omitempty"`
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
