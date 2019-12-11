package v1alpha1

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var _ webhook.Validator = &EtcdCluster{}

// ValidateCreate validates that all required fields are present and valid.
func (o *EtcdCluster) ValidateCreate() error {
	path := field.NewPath("spec")
	var allErrs field.ErrorList
	allErrs = append(
		allErrs,
		o.Spec.Storage.validate(path.Child("storage"))...,
	)
	return allErrs.ToAggregate()
}

// ValidateCreate validates that deletion is allowed
// TODO: Not yet implemented
func (o *EtcdCluster) ValidateDelete() error {
	var allErrs field.ErrorList
	return allErrs.ToAggregate()
}

// ValidateUpdate validates that only supported fields are changed
func (o *EtcdCluster) ValidateUpdate(old runtime.Object) error {
	oldO, ok := old.(*EtcdCluster)
	if !ok {
		return fmt.Errorf("Unexpected type for old: %#v", old)
	}
	oldO = oldO.DeepCopy()

	// Overwrite the fields which are allowed to change
	oldO.Spec.Replicas = o.Spec.Replicas

	if diff := cmp.Diff(oldO.Spec, o.Spec); diff != "" {
		return fmt.Errorf("Unsupported changes: (- current, + new) %s", diff)
	}
	return nil
}

var _ webhook.Validator = &EtcdPeer{}

// ValidateCreate validates that all required fields are present and valid.
func (o *EtcdPeer) ValidateCreate() error {
	path := field.NewPath("spec")
	var allErrs field.ErrorList

	allErrs = append(
		allErrs,
		o.Spec.Bootstrap.validate(path.Child("bootstrap"))...,
	)
	allErrs = append(
		allErrs,
		o.Spec.Storage.validate(path.Child("storage"))...,
	)
	return allErrs.ToAggregate()
}

// ValidateCreate validates that deletion is allowed
// TODO: Not yet implemented
func (o *EtcdPeer) ValidateDelete() error {
	var allErrs field.ErrorList
	return allErrs.ToAggregate()
}

// ValidateUpdate validates that only supported fields are changed
func (o *EtcdPeer) ValidateUpdate(old runtime.Object) error {
	oldO, ok := old.(*EtcdPeer)
	if !ok {
		return fmt.Errorf("Unexpected type for old: %#v", old)
	}
	oldO = oldO.DeepCopy()

	// Overwrite any the fields which are allowed to change
	// oldO.Spec.Foo = o.Spec.Foo

	if diff := cmp.Diff(oldO.Spec, o.Spec); diff != "" {
		return fmt.Errorf("Unsupported changes: (- current, + new) %s", diff)
	}
	return nil
}

func validatePersistentVolumeClaimSpec(path *field.Path, o *corev1.PersistentVolumeClaimSpec) field.ErrorList {
	var allErrs field.ErrorList
	if o == nil {
		allErrs = append(allErrs, field.Required(path, ""))
		return allErrs
	}
	if o.StorageClassName == nil {
		allErrs = append(allErrs, field.Required(path.Child("storageClassName"), ""))
		return allErrs
	}
	if _, ok := o.Resources.Requests["storage"]; !ok {
		allErrs = append(allErrs, field.Required(path.Child("resources", "requests", "storage"), ""))
		return allErrs
	}
	return allErrs
}

func (o *Bootstrap) validate(path *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if *o.InitialClusterState == InitialClusterStateNew && o.Static == nil {
		allErrs = append(allErrs, field.Required(path, "bootstrap.static must be supplied if initialClusterState: New"))
	}
	if *o.InitialClusterState == InitialClusterStateExisting && o.Static != nil {
		allErrs = append(allErrs, field.Forbidden(path, "bootstap.static must not be supplied if initialClusterState: Existing"))
	}
	return allErrs
}

func (o *EtcdPeerStorage) validate(path *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if o == nil {
		allErrs = append(allErrs, field.Required(path, ""))
		return allErrs
	}
	allErrs = append(
		allErrs,
		validatePersistentVolumeClaimSpec(path.Child("volumeClaimTemplate"), o.VolumeClaimTemplate)...,
	)
	return allErrs
}
