package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const DefaultSetSize = 256 // 256 uint64 to store a 4096 bitmap

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=vids;vidss,scope=Cluster
// +kubebuilder:printcolumn:name="CLUSTERNETWORK",type=string,JSONPath=`.spec.clusterNetwork`
// +kubebuilder:printcolumn:name="VLANCOUNT",type=string,JSONPath=`.spec.vlanCount`

type VlanIDSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VlanIDSetSpec `json:"spec"`
}

type VlanIDSetSpec struct {
	ClusterNetwork string   `json:"clusterNetwork"`
	VlanCount      uint32   `json:"vlanCount"`
	VIDSetHash     string   `json:"vidSetHash,omitempty"`
	VIDSet         []uint64 `json:"vidSet,omitempty"` // bitmap to store VID 1..4094
}
