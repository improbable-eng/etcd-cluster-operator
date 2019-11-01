package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EtcdClusterSpec defines the desired state of EtcdCluster
type EtcdClusterSpec struct {
	// Number of instances of etcd to assemble into this cluster
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Minimum=1
	Replicas *int32 `json:"replicas"`
	// Storage is the configuration of the disks and mount points of the Etcd
	// peers.
	Storage EtcdPeerStorage `json:"storage,omitempty"`
}

type EtcdMember struct {
	// Name is a human-readable name for the member. Will *typically* match the name we gave the peer that manages this
	// member.
	Name string `json:"name"`

	// ID is the internal unique identifier for the member that defines its identity with the etcd cluster. We do not
	// define this.
	ID string `json:"id"`
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
