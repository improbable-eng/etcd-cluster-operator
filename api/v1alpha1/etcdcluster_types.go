package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// EtcdClusterSpec defines the desired state of EtcdCluster
type EtcdClusterSpec struct {
	// Number of instances of etcd to assemble into this cluster
	Replicas *int32 `json:"replicas"`
	// Storage is the configuration of the disks and mount points of the Etcd
	// peers.
	Storage *EtcdPeerStorage `json:"storage,omitempty"`
}

type EtcdMember struct {
	Name string `json:"name"`
	ID   string `json:"ID"`
}

// EtcdClusterStatus defines the observed state of EtcdCluster
type EtcdClusterStatus struct {
	// Members contains information about each member from the etcd cluster.
	// +optional
	Members []EtcdMember `json:"members"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EtcdCluster is the Schema for the etcdclusters API
type EtcdCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdClusterSpec   `json:"spec,omitempty"`
	Status EtcdClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdClusterList contains a list of EtcdCluster
type EtcdClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdCluster{}, &EtcdClusterList{})
}
