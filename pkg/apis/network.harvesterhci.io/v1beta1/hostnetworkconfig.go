package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=hnc;hncs,scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:validation:XValidation:rule="!(has(self.spec.ips) && has(self.spec.hostMultiFamilyIPs))",message="Fields 'ips' and 'hostMultiFamilyIPs' are mutually exclusive; use 'hostMultiFamilyIPs' for advanced configurations."
// +kubebuilder:validation:XValidation:rule="self.spec.mode != 'static' || (has(self.spec.ips) && size(self.spec.ips) > 0) || (has(self.spec.hostMultiFamilyIPs) && size(self.spec.hostMultiFamilyIPs) > 0)",message="Either 'ips' or 'hostMultiFamilyIPs' must be specified and non-empty when mode is static"
// +kubebuilder:validation:XValidation:rule="(has(oldSelf.spec.ipFamily) ? oldSelf.spec.ipFamily : 'ipv4-only') == (has(self.spec.ipFamily) ? self.spec.ipFamily : 'ipv4-only')",message="spec.ipFamily is immutable after creation"
// +kubebuilder:validation:XValidation:rule="oldSelf.spec.vlanID == self.spec.vlanID",message="spec.vlanID is immutable after creation"
// +kubebuilder:validation:XValidation:rule="oldSelf.spec.clusterNetwork == self.spec.clusterNetwork",message="spec.clusterNetwork is immutable after creation"

type HostNetworkConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              HostNetworkConfigSpec `json:"spec"`
	// +optional
	Status HostNetworkConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:MaxLength=64
// +kubebuilder:validation:XValidation:rule="isCIDR(self)",message="Invalid CIDR format"
type IPAddr string

type HostNetworkConfigSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	// Optional: select target nodes by labels
	// If empty, applies to all nodes
	// +optional
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	// Required, non-empty
	// +kubebuilder:validation:MinLength=1
	ClusterNetwork string `json:"clusterNetwork"`

	// +optional
	Underlay bool `json:"underlay"`

	// Required, must be 1-4094
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4094
	VlanID uint16 `json:"vlanID"`

	// Required, non-empty, only 'static' or 'dhcp'
	// +kubebuilder:validation:Enum=static;dhcp
	// +kubebuilder:validation:MinLength=1
	Mode string `json:"mode"`

	// IP Family configuration (defaults to ipv4-only if omitted)
	// +optional
	// +kubebuilder:validation:Enum=ipv4-only;ipv6-only;dual-stack
	// +kubebuilder:default=ipv4-only
	IPFamily string `json:"ipFamily,omitempty"`

	// Optional default gateway for IPv4 (applies to all nodes)
	// +optional
	GatewayIPv4 *string `json:"gatewayIPv4,omitempty"`

	// Optional default gateway for IPv6 (applies to all nodes)
	// +optional
	GatewayIPv6 *string `json:"gatewayIPv6,omitempty"`

	// Optional map, validated if mode=static (Legacy IPv4-only)
	// +optional
	// +kubebuilder:validation:MaxProperties=500
	HostIPs map[string]IPAddr `json:"ips,omitempty"`

	// Optional structured per-node configuration (Supports multi-family)
	// +optional
	// +kubebuilder:validation:MaxProperties=500
	HostMultiFamilyIPs map[string]MultiFamilyIPConfig `json:"hostMultiFamilyIPs,omitempty"`
}

// MultiFamilyIPConfig holds multi-family IP configurations per host.
type MultiFamilyIPConfig struct {
	// Optional unique IPv4 address and subnet mask for this node
	// +optional
	// +kubebuilder:validation:XValidation:rule="cidr(string(self)).ip().family() == 4", message="ipv4 must be a valid IPv4 address/CIDR"
	IPv4 *IPAddr `json:"ipv4,omitempty"`

	// Optional unique IPv6 address and subnet mask for this node
	// +optional
	// +kubebuilder:validation:XValidation:rule="cidr(string(self)).ip().family() == 6", message="ipv6 must be a valid IPv6 address/CIDR"
	IPv6 *IPAddr `json:"ipv6,omitempty"`
}

type HostNetworkConfigStatus struct {
	// global observed state
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
	// Per-node observed state
	// key = node name
	// +optional
	NodeStatus map[string]HostNetworkConfigNodeStatus `json:"nodeStatus,omitempty"`
}

type HostNetworkConfigNodeStatus struct {
	// Mode static or dhcp. If empty (e.g., on upgrade paths), fallback to spec.mode.
	// +optional
	Mode string `json:"mode,omitempty"`

	// Applied IP family. If empty (e.g., on upgrade paths), fallback to spec.ipFamily.
	// +optional
	IPFamily string `json:"ipFamily,omitempty"`

	// Observed and active network configuration (records static config or active DHCP lease)
	// +optional
	Network *NodeNetworkState `json:"network,omitempty"`

	// Node-specific conditions
	Conditions []Condition `json:"conditions,omitempty"`
}

type NodeNetworkState struct {
	// Active IPv4 address
	// +optional
	IPv4 *string `json:"ipv4,omitempty"`

	// Active IPv6 address
	// +optional
	IPv6 *string `json:"ipv6,omitempty"`

	// Active IPv4 gateway
	// +optional
	GatewayIPv4 *string `json:"gatewayIPv4,omitempty"`

	// Active IPv6 gateway
	// +optional
	GatewayIPv6 *string `json:"gatewayIPv6,omitempty"`
}
