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
	// ObjectURLTemplate is a URL of a file of a backup in object storage.
	//
	// It *MAY* contain go-template style template fields.
	// The fields *MUST* match fields the EtcdBackup resource.
	// For example:
	//  s3://example-bucket/snapshot.db
	//  s3://example-bucket/{{ .Namespace }}/{{ .Name }}/{{ .CreationTimestamp }}/snapshot.db
	//
	// You *SHOULD* include template fields if the URL will be used in an EtcdBackupSchedule,
	// to ensure that every backup has a unique name.
	// For example:
	//  s3://example-bucket/snapshot-{{ .UID }}.db
	//
	// The scheme of this URL should be gs:// or s3://.
	ObjectURLTemplate string `json:"objectURLTemplate"`
}

type EtcdBackupSource struct {
	// ClusterURL is a URL endpoint for a single Etcd server.
	// The etcd-cluster-operator backup-agent connects to this endpoint,
	// downloads a snapshot from remote etcd server and uploads the data to
	// EtcdBackup.Destination.ObjectURLTemplate.
	// The Etcd Snapshot API works with a single selected node, and the saved
	// snapshot is the point-in-time state of that selected node.
	// See https://github.com/etcd-io/etcd/blob/v3.4.4/clientv3/snapshot/v3_snapshot.go#L53
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
