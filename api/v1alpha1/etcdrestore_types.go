package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EtcdRestoreSource struct {

	// Note: This structure requires that you provide a `bucket`. But is designed such that you could modify this struct
	//       in the future to add a new way to get the snapshot data. This would require a code change but would not
	//       require a change in existing YAML files that use the current `restore.spec.source.bucket` path.

	// Bucket identifies a generic blob Storage bucket to pull the snapshot from.
	Bucket EtcdRestoreSourceBucket `json:"bucket"`
}

type EtcdRestoreSourceBucket struct {
	// BucketURL is the name of the storage bucket. This is a go-cloud bucket URL https://gocloud.dev/howto/blob/ and
	// should use a URL scheme of the bucket provider. For example `s3://my-amazon-bucket` or
	// `gs://my-google-cloud-bucket`.
	// +kubebuilder:validation:MinLength=3
	// +kubebuilder:validation:MaxLength=222
	BucketURL string `json:"bucketURL"`

	// ObjectPath is the path to the object inside the bucket.
	// +kubebuilder:validation:MinLength=1
	ObjectPath string `json:"objectPath"`

	// Credentials holds the method of obtaining credentials that will be passed to the storage provider.
	// +optional
	Credentials *BucketCredentials `json:"credentials,omitempty"`
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
