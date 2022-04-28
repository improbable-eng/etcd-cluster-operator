package controllers

import (
	"context"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/reconcilerevent"
)

// EtcdBackupReconciler reconciles a EtcdBackup object
type EtcdBackupReconciler struct {
	client.Client
	Log              logr.Logger
	Scheme           *runtime.Scheme
	BackupAgentImage string
	ProxyURL         string
	Recorder         record.EventRecorder
}

// backupState is a container for the Backup, the actual status of its
// dependencies and the desired dependencies.
// It has all the state necessary in deciding what action to perform next.
type backupState struct {
	backup  *etcdv1alpha1.EtcdBackup
	actual  *backupStateContainer
	desired *backupStateContainer
}

// backupStateContainer is a container for the dependencies of the EtcdBackup.
type backupStateContainer struct {
	serviceAccount *corev1.ServiceAccount
	pod            *corev1.Pod
}

// setStateActual populates backupState.actual by making read-only API requests.
func (r *EtcdBackupReconciler) setStateActual(ctx context.Context, state *backupState) error {
	var actual backupStateContainer

	key := client.ObjectKey{
		Name:      state.backup.Name,
		Namespace: state.backup.Namespace,
	}

	actual.serviceAccount = &corev1.ServiceAccount{}
	if err := r.Get(ctx, key, actual.serviceAccount); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("error while getting service account: %s", err)
		}
		actual.serviceAccount = nil
	}
	actual.pod = &corev1.Pod{}
	if err := r.Get(ctx, key, actual.pod); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("error while getting pod: %s", err)
		}
		actual.pod = nil
	}

	state.actual = &actual
	return nil
}

// setStateDesired populates the backupState.desired.
// It computes these resources based on the supplied backupState.backup
// And it should set a controller reference for each desired resource so that
// they will be garbage collected when the EtcdBack is deleted.
func (r *EtcdBackupReconciler) setStateDesired(state *backupState) error {
	var desired backupStateContainer

	desired.serviceAccount = serviceAccountForBackup(state.backup)
	if err := controllerutil.SetControllerReference(state.backup, desired.serviceAccount, r.Scheme); err != nil {
		return fmt.Errorf("error setting service account controller reference: %s", err)
	}

	pod, err := podForBackup(state.backup, r.BackupAgentImage, r.ProxyURL, desired.serviceAccount.Name)
	if err != nil {
		return fmt.Errorf("error %q computing pod for backup", err)
	}
	if err := controllerutil.SetControllerReference(state.backup, pod, r.Scheme); err != nil {
		return fmt.Errorf("error setting pod controller reference: %s", err)
	}
	desired.pod = pod
	state.desired = &desired
	return nil
}

// getState creates a backupState by making read-only API requests.
// The returned state is used later to calculate the next action to perform.
func (r EtcdBackupReconciler) getState(ctx context.Context, req ctrl.Request) (*backupState, error) {
	var state backupState

	state.backup = &etcdv1alpha1.EtcdBackup{}
	if err := r.Get(ctx, req.NamespacedName, state.backup); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("error while getting backup: %s", err)
		}
		state.backup = nil
		return &state, nil
	}

	if err := r.setStateActual(ctx, &state); err != nil {
		return nil, fmt.Errorf("error setting actual state: %s", err)
	}

	if err := r.setStateDesired(&state); err != nil {
		return nil, fmt.Errorf("error setting desired state: %s", err)
	}

	return &state, nil
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdbackups,verbs=get;list;watch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create

func (r *EtcdBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("etcdbackup-name", req.NamespacedName)

	// Get the state of the EtcdBackup and all its dependencies.
	state, err := r.getState(ctx, req)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error getting state: %s", err)
	}

	// Calculate a single action to perform next, based on the state.
	var (
		action Action
		event  reconcilerevent.ReconcilerEvent
	)
	switch {
	case state.backup == nil:
		log.Info("Backup resource not found. Ignoring.")
	case !state.backup.DeletionTimestamp.IsZero():
		log.Info("Backup resource has been deleted. Ignoring.")
	case state.backup.Status.Phase == "":
		log.Info("Backup starting. Updating status.")
		new := state.backup.DeepCopy()
		new.Status.Phase = etcdv1alpha1.EtcdBackupPhaseBackingUp
		action = &PatchStatus{client: r.Client, original: state.backup, new: new}
	case state.backup.Status.Phase == etcdv1alpha1.EtcdBackupPhaseFailed:
		log.Info("Backup has failed. Ignoring.")
	case state.backup.Status.Phase == etcdv1alpha1.EtcdBackupPhaseCompleted:
		log.Info("Backup has completed. Ignoring.")
	case state.actual.serviceAccount == nil:
		log.Info("Service account does not exist. Creating.")
		action = &CreateRuntimeObject{client: r.Client, obj: state.desired.serviceAccount}
	case state.actual.pod == nil:
		log.Info("Pod does not exist. Creating.")
		action = &CreateRuntimeObject{client: r.Client, obj: state.desired.pod}
	case state.actual.pod.Status.Phase == corev1.PodFailed:
		log.Info("Backup agent failed. Updating status.")
		new := state.backup.DeepCopy()
		new.Status.Phase = etcdv1alpha1.EtcdBackupPhaseFailed
		action = &PatchStatus{client: r.Client, original: state.backup, new: new}
		event = &reconcilerevent.BackupFailed{For: state.backup}
	case state.actual.pod.Status.Phase == corev1.PodSucceeded:
		log.Info("Backup agent succeeded. Updating status.")
		new := state.backup.DeepCopy()
		new.Status.Phase = etcdv1alpha1.EtcdBackupPhaseCompleted
		action = &PatchStatus{client: r.Client, original: state.backup, new: new}
		event = &reconcilerevent.BackupSucceeded{For: state.backup}
	}

	// Execute the action
	if action != nil {
		err := action.Execute(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("error executing action: %s", err)
		}
		// Generate a generic event for CreateRuntimeObject actions
		if event == nil {
			switch o := action.(type) {
			case *CreateRuntimeObject:
				event = &reconcilerevent.ObjectCreatedEvent{Log: log, For: state.backup, Object: o.obj}
			}
		}
	}

	// Record an event
	if event != nil {
		event.Record(r.Recorder)
	}

	return ctrl.Result{}, nil
}

func (r *EtcdBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdBackup{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.Pod{}).
		Complete(r)
}

// serviceAccountForBackup creates a restriced service-account by the backup-agent pod.
func serviceAccountForBackup(backup *etcdv1alpha1.EtcdBackup) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
	}
}

// podForBackup creates a pod for running the backup-agent image.
// It does not need to interact with the API and should not have permissions to
// do so.
func podForBackup(backup *etcdv1alpha1.EtcdBackup, image, proxyURL, serviceAccount string) (*corev1.Pod, error) {
	tmpl, err := template.New("template").Parse(backup.Spec.Destination.ObjectURLTemplate)
	if err != nil {
		return nil, fmt.Errorf("error %q parsing object URL template", err)
	}
	var objectURL strings.Builder
	if err := tmpl.Execute(&objectURL, backup); err != nil {
		return nil, fmt.Errorf("error %q executing template", err)
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: serviceAccount,
			Containers: []corev1.Container{
				{
					Name:  "backup-agent",
					Image: image,
					Args: []string{
						"--proxy-url", proxyURL,
						"--backup-url", objectURL.String(),
						"--etcd-url", backup.Spec.Source.ClusterURL,
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("50Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("50Mi"),
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}, nil
}
