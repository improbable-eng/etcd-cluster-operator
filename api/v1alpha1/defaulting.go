package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	defaultVolumeMode = corev1.PersistentVolumeFilesystem
)

var _ webhook.Defaulter = &EtcdCluster{}

// Default sets default values for optional EtcdPeer fields.
// This is used in webhooks and in the Reconciler to ensure that nil pointers
// have been replaced with concrete pointers.
// This avoids nil pointer panics later on.
func (o *EtcdCluster) Default() {
	if o == nil {
		return
	}
	o.Spec.Storage.setDefaults()
}

var _ webhook.Defaulter = &EtcdPeer{}

// Default sets default values for optional EtcdPeer fields.
// This is used in webhooks and in the Reconciler to ensure that nil pointers
// have been replaced with concrete pointers.
// This avoids nil pointer panics later on.
func (o *EtcdPeer) Default() {
	if o == nil {
		return
	}
	o.Spec.Storage.setDefaults()
}

func (o *EtcdPeerStorage) setDefaults() {
	if o == nil {
		return
	}
	if o.VolumeClaimTemplate.AccessModes == nil {
		o.VolumeClaimTemplate.AccessModes = []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		}
	}

	if o.VolumeClaimTemplate.VolumeMode == nil {
		o.VolumeClaimTemplate.VolumeMode = &defaultVolumeMode
	}
}
