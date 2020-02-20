package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EtcdBackupPhase string

var (
	EtcdBackupPhaseBackingUp EtcdBackupPhase = "BackingUp"
	EtcdBackupPhaseCompleted EtcdBackupPhase = "Completed"
	EtcdBackupPhaseFailed    EtcdBackupPhase = "Failed"
)

// EtcdBackupDestination holds a storage location where an etcd backup will be placed.
type EtcdBackupDestination struct {
	// ObjectURL is a URL of a file of a backup in object storage.
	// The scheme of this URL should be gs:// or s3://.
	ObjectURL string `json:"objectURL"`
}

type EtcdBackupSource struct {
	// ClusterURL is a URL endpoint for the Etcd cluster
	ClusterURL string `json:"clusterURL"`
}

// EtcdBackupSpec defines the desired state of EtcdBackup
type EtcdBackupSpec struct {
	// Source describes the cluster to be backed up.
	Source EtcdBackupSource `json:"source"`
	// Destination is the remote location where the backup will be placed.
	Destination EtcdBackupDestination `json:"destination"`
}

// EtcdBackupStatus defines the observed state of EtcdBackup
type EtcdBackupStatus struct {
	// Phase defines the current operation that the backup process is taking.
	Phase EtcdBackupPhase `json:"phase,omitempty"`
	// StartTime is the times that this backup entered the `BackingUp' phase.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// CompletionTime is the time that this backup entered the `Completed' phase.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EtcdBackup is the Schema for the etcdbackups API
type EtcdBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdBackupSpec   `json:"spec,omitempty"`
	Status EtcdBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdBackupList contains a list of EtcdBackup
type EtcdBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdBackup{}, &EtcdBackupList{})
}
