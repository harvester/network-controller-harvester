package v1alpha1

import (
	"github.com/rancher/wrangler/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=hn;hns,scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="DESCRIPTION",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="TYPE",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="NIC",type=string,JSONPath=`.spec.nic`

type HostNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostNetworkSpec   `json:"spec,omitempty"`
	Status HostNetworkStatus `json:"status,omitempty"`
}

type HostNetworkSpec struct {
	// +optional
	Description string `json:"description,omitempty"`

	// +optional
	Type NetworkType `json:"type,omitempty"`

	// +optional
	NIC string `json:"nic,omitempty"`
}

// +kubebuilder:validation:Enum=vlan
type NetworkType string

const (
	NetworkTypeVLAN NetworkType = "vlan"
)

type HostNetworkStatus struct {
	// +optional
	NetworkIDs []NetworkID `json:"networkIDs,omitempty"`

	// +optional
	NetworkLinkStatus map[string]*LinkStatus `json:"networkLinkStatus,omitempty"`

	// +optional
	PhysicalNics []PhysicalNic `json:"physicalNics,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

type PhysicalNic struct {
	Index               int    `json:"index,omitempty"`
	Name                string `json:"name,omitempty"`
	UsedByManageNetwork bool   `json:"usedByManageNetwork,omitempty"`
}

type NetworkID int

type LinkStatus struct {
	// +optional
	Index int `json:"index,omitempty"`

	// +optional
	Type string `json:"type,omitempty"`

	// +optional
	MAC string `json:"mac,omitempty"`

	// +optional
	Promiscuous bool `json:"promiscuous,omitempty"`

	// +optional
	State string `json:"state,omitempty"`

	// +optional
	IPV4Address []string `json:"ipv4Address,omitempty"`

	// +optional
	Master string `json:"master,omitempty"`

	// +optional
	Routes []string `json:"routes,omitempty"`

	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

var (
	HostNetworkReady           condition.Cond = "Ready"
	HostNetworkIDConfigured    condition.Cond = "NetworkIDConfigured"
	HostNetworkLinkReady       condition.Cond = "LinkReady"
	HostNetworkRouteConfigured condition.Cond = "RouteConfigured"
)

type Condition struct {
	// Type of the condition.
	Type condition.Cond `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime string `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}
