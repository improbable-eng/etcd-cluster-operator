package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type EtcdBackupPhase string

var (
	EtcdBackupPhaseBackingUp EtcdBackupPhase = "BackingUp"
	EtcdBackupPhaseUploading EtcdBackupPhase = "Uploading"
	EtcdBackupPhaseCompleted EtcdBackupPhase = "Completed"
	EtcdBackupPhaseFailed    EtcdBackupPhase = "Failed"
)

// EtcdClusterEndpoint holds an addressable endpoint for an etcd member.
type EtcdClusterEndpoint struct {
	// Port that is exposing the etcd client API for this member.
	Port int `json:"port"`
	// An IP address or DNS name of an endpoint.
	Host string `json:"host"`
	// Scheme to use for connecting to the host.
	Scheme corev1.URIScheme `json:"scheme"`
}

// EtcdBackupDestination holds a storage location where an etcd backup will be placed.
// At most one destination must be set.
type EtcdBackupDestination struct {
	// Local, when set, will copy the backup into a local volume on the pod taking the backup.
	// +optional
	Local *EtcdBackupDestinationLocal `json:"local,omitempty"`
	// GCSBucket, when set, will push the backup to a Google Cloud Storage bucket.
	// +optional
	GCSBucket *EtcdBackupDestinationGCSBucket `json:"gcsBucket,omitempty"`
}

// EtcdBackupDestinationLocal describes a local directory into which to put the backup file. This is in the
// filesystem of the pod running the operator. To persist this between pod restarts, ensure that path is
// inside a mounted volume.
type EtcdBackupDestinationLocal struct {
	// Directory is an absolute filepath to a directory where backups will be placed.
	Directory string `json:"path"`
}

// EtcdBackupDestinationGCSBucket describes a remote storage bucket on Google Cloud Storage, and the mechanisms used
// to access this bucket.
type EtcdBackupDestinationGCSBucket struct {
	// BucketName is the name of the storage bucket.
	//+kubebuilder:validation:MinLength=3
	//+kubebuilder:validation:MaxLength=222
	BucketName string `json:"bucketName"`
	// Credentials holds the method of obtaining credentials that will be provided to the
	// Google Cloud APIs in order to write backup data.
	// +optional
	Credentials *GoogleCloudCredentials `json:"credentials,omitempty"`
}

type GoogleCloudCredentials struct {
	// Credentials are taken from the key of a Kubernetes secret.
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeySelector,omitempty"`
}

// EtcdBackupSpec defines the desired state of EtcdBackup
type EtcdBackupSpec struct {
	// ClusterEndpoints holds one or more endpoints fronting etcd's gRPC API.
	// Multiple endpoints may only be supported by some backup types.
	ClusterEndpoints []EtcdClusterEndpoint `json:"clusterEndpoints"`
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
	// BackupPath is the path to the final backup file inside of the destination.
	// The format of this string varies based on the destination.
	BackupPath string `json:"backupPath,omitempty"`
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
