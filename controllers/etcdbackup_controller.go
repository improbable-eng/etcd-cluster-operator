package controllers

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/backup"
)

// EtcdBackupReconciler reconciles a EtcdBackup object
type EtcdBackupReconciler struct {
	client.Client
	Log logr.Logger

	// An absolute path to a directory which will contain backups that have not yet been pushed to their destination.
	TempDir string
}

func (r *EtcdBackupReconciler) reconcile(
	ctx context.Context,
	resourceID string,
	resource *etcdv1alpha1.EtcdBackup) (
	nextPhase *etcdv1alpha1.EtcdBackupPhase,
	remoteBackupPath string,
	err error) {

	// If status is not yet set, the resource was probably just created.
	if resource.Status.Phase == "" {
		// Mark the backup as started.
		return &etcdv1alpha1.EtcdBackupPhaseBackingUp, "", nil
	}

	// The backup will live on disk in a well-known location, unique to the resource describing this backup.
	localPath := filepath.Join(r.TempDir, string(resource.UID), "snapshot.db")

	// Extract backup method config.
	endpoints := make([]string, len(resource.Spec.ClusterEndpoints))
	for i, e := range resource.Spec.ClusterEndpoints {
		endpoints[i] = (&url.URL{
			Scheme: strings.ToLower(string(e.Scheme)),
			Host:   fmt.Sprintf("%s:%d", e.Host, e.Port),
		}).String()
	}
	method := &backup.SnapshotMethod{
		Endpoints: endpoints,
	}

	// If the backup has finished, or failed, make sure there are no artifacts remaining local to the operator.
	if resource.Status.Phase == etcdv1alpha1.EtcdBackupPhaseCompleted || resource.Status.Phase == etcdv1alpha1.EtcdBackupPhaseFailed {
		// Clean the backup from the local disk.
		err = method.Delete(ctx, localPath)
		if err != nil {
			r.Log.Error(err, "Failed to remove the backup from local disk")
			// No error is returned here - the backup was successfully completed but the cleanup failed.
		}
		return nil, "", nil
	}

	// The the backup does not already exist on disk, take it.
	if saved, err := method.IsSaved(ctx, localPath); err != nil {
		r.Log.Error(err, "Failed to check if the backup already exists on disk")
		return &etcdv1alpha1.EtcdBackupPhaseFailed, "", err
	} else if !saved {
		err := method.Save(ctx, localPath)
		if err != nil {
			r.Log.Error(err, "Failed to save the backup")
			return &etcdv1alpha1.EtcdBackupPhaseFailed, "", err
		}
		return &etcdv1alpha1.EtcdBackupPhaseUploading, "", nil
	}

	// The backup will live at this path in the remote storage location.
	remoteFileName := resource.Status.StartTime.Format(time.RFC3339) + ".db"

	// The backup now exists on disk, extract the destination to send it.
	d := resource.Spec.Destination
	var dest backup.Destination
	if d.Local != nil {
		dest = &backup.LocalVolumeDestination{
			Path: filepath.Join(d.Local.Directory, remoteFileName),
		}
	}
	if d.GCSBucket != nil {
		dest = &backup.GCSDestination{
			BlobName: remoteFileName,
			Bucket:   d.GCSBucket.BucketName,
		}
	}
	if dest == nil {
		return &etcdv1alpha1.EtcdBackupPhaseFailed, "", fmt.Errorf("missing destination")
	}

	remoteStoredPath, err := dest.RemotePath()
	if err != nil {
		r.Log.Error(err, "Failed to compute the remote destination for the backup to be placed")
		return &etcdv1alpha1.EtcdBackupPhaseFailed, "", err
	}

	// Check if it has been sent to the destination already.
	if uploaded, err := dest.IsUploaded(ctx); err != nil {
		r.Log.Error(err, "Failed to check if the backup has already been uploaded")
		return &etcdv1alpha1.EtcdBackupPhaseFailed, "", err
	} else if !uploaded {
		// Upload the backup.
		err := dest.Upload(ctx, localPath)
		if err != nil {
			r.Log.Error(err, "Failed to upload the backup")
			return &etcdv1alpha1.EtcdBackupPhaseFailed, "", err
		}
		return &etcdv1alpha1.EtcdBackupPhaseCompleted, remoteStoredPath, nil
	}

	// Flow should not reach here, but if it does it means that the backup has already been uploaded.
	return &etcdv1alpha1.EtcdBackupPhaseCompleted, remoteStoredPath, nil
}

func (r *EtcdBackupReconciler) updateStatus(
	ctx context.Context,
	resourceID string,
	resource *etcdv1alpha1.EtcdBackup,
	nextPhase *etcdv1alpha1.EtcdBackupPhase,
	remotePath string) error {

	if nextPhase == nil {
		return nil
	}

	switch *nextPhase {
	case etcdv1alpha1.EtcdBackupPhaseBackingUp:
		resource.Status.Phase = etcdv1alpha1.EtcdBackupPhaseBackingUp
		resource.Status.StartTime = &metav1.Time{
			Time: time.Now(),
		}
	case etcdv1alpha1.EtcdBackupPhaseUploading:
		resource.Status.Phase = etcdv1alpha1.EtcdBackupPhaseUploading
	case etcdv1alpha1.EtcdBackupPhaseCompleted:
		resource.Status.Phase = etcdv1alpha1.EtcdBackupPhaseCompleted
		resource.Status.CompletionTime = &metav1.Time{
			Time: time.Now(),
		}
		resource.Status.BackupPath = remotePath
	case etcdv1alpha1.EtcdBackupPhaseFailed:
		resource.Status.Phase = etcdv1alpha1.EtcdBackupPhaseFailed
	default:
		return fmt.Errorf("unknown next backup phase '%v'", nextPhase)
	}

	if err := r.Client.Status().Update(ctx, resource); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdbackups,verbs=get;list;watch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdbackups/status,verbs=get;update;patch

func (r *EtcdBackupReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("backup", req.NamespacedName)

	resource := &etcdv1alpha1.EtcdBackup{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	nextPhase, remotePath, reconcileErr := r.reconcile(ctx, req.NamespacedName.String(), resource)
	if reconcileErr != nil {
		log.Error(reconcileErr, "Failed to reconcile")
	}

	updateStatusErr := r.updateStatus(ctx, req.NamespacedName.String(), resource, nextPhase, remotePath)
	if updateStatusErr != nil {
		log.Error(updateStatusErr, "Failed to update status")
	}

	return ctrl.Result{}, k8serrors.NewAggregate([]error{reconcileErr, updateStatusErr})
}

func (r *EtcdBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdBackup{}).
		Complete(r)
}
