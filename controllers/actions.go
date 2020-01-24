package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

type Action interface {
	Execute(context.Context) error
}

type NoopAction struct {
}

func (_ *NoopAction) Execute(_ context.Context) error {
	return nil
}

type CreateRuntimeObject struct {
	log    logr.Logger
	client client.Client
	obj    runtime.Object
}

func (o *CreateRuntimeObject) Execute(ctx context.Context) error {
	o.log.Info("Creating resource", "type", o.obj.GetObjectKind().GroupVersionKind().String())
	err := o.client.Create(ctx, o.obj)
	if apierrs.IsAlreadyExists(err) {
		err = fmt.Errorf("stale cache error: object was not found in cache but creation failed with AlreadyExists error: %s", err)
	}
	return err
}

// PeerPVCDeleter deletes the PVC for an EtcdPeer and removes the PVC deletion
// finalizer.
type PeerPVCDeleter struct {
	log    logr.Logger
	client client.Client
	peer   *etcdv1alpha1.EtcdPeer
}

// Execute performs the deletiong and finalizer removal
func (o *PeerPVCDeleter) Execute(ctx context.Context) error {
	o.log.V(2).Info("Deleting PVC for peer prior to deletion")
	expectedPvc := pvcForPeer(o.peer)
	expectedPvcNamespacedName, err := client.ObjectKeyFromObject(expectedPvc)
	if err != nil {
		return fmt.Errorf("unable to get ObjectKey from PVC: %s", err)
	}
	var actualPvc corev1.PersistentVolumeClaim
	err = o.client.Get(ctx, expectedPvcNamespacedName, &actualPvc)
	switch {
	case err == nil:
		// PVC exists.
		// Check whether it has already been deleted (probably by us).
		// It won't actually be deleted until the garbage collector
		// deletes the Pod which is using it.
		if actualPvc.ObjectMeta.DeletionTimestamp.IsZero() {
			o.log.V(2).Info("Deleting PVC for peer")
			err := o.client.Delete(ctx, expectedPvc)
			if err == nil {
				o.log.V(2).Info("Deleted PVC for peer")
				return nil
			}
			return fmt.Errorf("failed to delete PVC for peer: %w", err)
		}
		o.log.V(2).Info("PVC for peer has already been marked for deletion")

	case apierrors.IsNotFound(err):
		o.log.V(2).Info("PVC not found for peer. Already deleted or never created.")

	case err != nil:
		return fmt.Errorf("failed to get PVC for deleted peer: %w", err)

	}

	// If we reach this stage, the PVC has been deleted or didn't need
	// deleting.
	// Remove the finalizer so that the EtcdPeer can be garbage
	// collected along with its replicaset, pod...and with that the PVC
	// will finally be deleted by the garbage collector.
	o.log.V(2).Info("Removing PVC cleanup finalizer")
	updated := o.peer.DeepCopy()
	controllerutil.RemoveFinalizer(updated, pvcCleanupFinalizer)
	if err := o.client.Patch(ctx, updated, client.MergeFrom(o.peer)); err != nil {
		return fmt.Errorf("failed to remove PVC cleanup finalizer: %w", err)
	}
	o.log.V(2).Info("Removed PVC cleanup finalizer")
	return nil
}
