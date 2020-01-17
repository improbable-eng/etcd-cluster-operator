package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// Structs in here are shared between EtcdBackup and EtcdRestore

// Credentials for bucket access.
type BucketCredentials struct {
	// Provide credentials for Google Cloud Platform.
	// +optional
	GoogleCloud *GoogleCloudCredentials `json:"googleCloud,omitempty"`
}

// GoogleCloudCredentials defines a reference to a key to authenticate against Google Cloud Storage. The user is
// responsible for ensuring that the referenced Kubernetes Secret exists, that it contains a key, that the key is valid
// for the Google Cloud Storage Bucket they are trying to access, and that it has appropriate permissions for the
// operation at hand (write for backup, read for restore).
type GoogleCloudCredentials struct {
	// Credentials are taken from the key of a Kubernetes secret.
	SecretKeyRef *corev1.SecretKeySelector `json:"secretKeySelector,omitempty"`
}
