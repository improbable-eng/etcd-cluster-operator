package reconcilerevent

import (
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = etcdv1alpha1.AddToScheme(scheme)

}

// ObjectCreatedEvent is recorded each time the controller successfully creates
// a new API resource.
// It performs runtime inspection of the supplied object in order to log kind,
// group and name of the object.
type ObjectCreatedEvent struct {
	Log    logr.Logger
	For    runtime.Object
	Object runtime.Object
}

func (o *ObjectCreatedEvent) Record(recorder record.EventRecorder) {
	gvk, err := apiutil.GVKForObject(o.Object, scheme)
	if err != nil {
		o.Log.Error(err, "Failure accessing GVK", "object", o.Object)
		return
	}
	objMeta, err := meta.Accessor(o.Object)
	if err != nil {
		o.Log.Error(err, "Failure accessing metadata", "object", o.Object)
		return
	}

	recorder.Event(
		o.For,
		K8sEventTypeNormal,
		"SuccessfulCreate",
		fmt.Sprintf("Created %s: %s", gvk.Kind, objMeta.GetName()),
	)
}

// BackupSucceeded is an event generated when the backup has succeeded.
type BackupSucceeded struct {
	For runtime.Object
}

func (o *BackupSucceeded) Record(recorder record.EventRecorder) {
	recorder.Event(
		o.For,
		v1.EventTypeNormal,
		etcdv1alpha1.EtcdBackupReasonSucceeded,
		"Backup completed successfully",
	)
}

// BackupFailed is an event generated when the backup has failed.
type BackupFailed struct {
	For runtime.Object
}

func (o *BackupFailed) Record(recorder record.EventRecorder) {
	recorder.Event(
		o.For,
		v1.EventTypeWarning,
		etcdv1alpha1.EtcdBackupReasonFailed,
		"Backup failed. See backup-agent pod logs for details.",
	)
}
