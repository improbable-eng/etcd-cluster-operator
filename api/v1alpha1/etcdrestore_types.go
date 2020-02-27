package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EtcdRestoreSource struct {
	// ObjectURL is a URL of a file of a backup in object storage.
	// The scheme of this URL should be gs:// or s3://.
	ObjectURL string `json:"objectURL"`
}

// A template to define the cluster we'll make. The namespace will be the same as this restore resource.
type EtcdClusterTemplate struct {
	// ClusterName is the name of the EtcdCluster that will be created by this operation.
	ClusterName string `json:"clusterName"`

	// Spec is the specification of the cluster that will be created by this operation.
	Spec EtcdClusterSpec `json:"spec"`
}

// EtcdRestoreSpec defines the desired state of EtcdRestore
type EtcdRestoreSpec struct {
	// Source describes the location the backup is pulled from
	Source EtcdRestoreSource `json:"source"`

	// ClusterTemplate describes the EtcdCluster that will eventually exist
	ClusterTemplate EtcdClusterTemplate `json:"clusterTemplate"`
}

type EtcdRestorePhase string

var (
	EtcdRestorePhasePending   EtcdRestorePhase = "Pending"
	EtcdRestorePhaseFailed    EtcdRestorePhase = "Failed"
	EtcdRestorePhaseCompleted EtcdRestorePhase = "Completed"
)

// EtcdRestoreStatus defines the observed state of EtcdRestore
type EtcdRestoreStatus struct {
	// Phase is what the restore is doing last time it was checked. The possible end states are "Failed" and
	// "Completed".
	Phase EtcdRestorePhase `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EtcdRestore is the Schema for the etcdrestores API
type EtcdRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdRestoreSpec   `json:"spec,omitempty"`
	Status EtcdRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdRestoreList contains a list of EtcdRestore
type EtcdRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdRestore{}, &EtcdRestoreList{})
}
