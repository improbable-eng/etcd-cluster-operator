package controllers

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

type CronScheduler interface {
	AddFunc(spec string, cmd func()) (cron.EntryID, error)
	Remove(id cron.EntryID)
}

// EtcdBackupScheduleReconciler reconciles a EtcdBackupSchedule object
type EtcdBackupScheduleReconciler struct {
	client.Client
	Log logr.Logger

	// CronHandler is able to schedule cronjobs to occur at given times.
	CronHandler CronScheduler

	// Schedules holds a mapping of resources to the object responsible for scheduling the backup to be taken.
	Schedules *ScheduleMap
}

type Schedule struct {
	cronEntry cron.EntryID
}

// ScheduleMap is a thread-safe mapping of backup schedules.
type ScheduleMap struct {
	sync.RWMutex
	data map[string]Schedule
}

func NewScheduleMap() *ScheduleMap {
	return &ScheduleMap{
		RWMutex: sync.RWMutex{},
		data:    map[string]Schedule{},
	}
}

func (s *ScheduleMap) Read(key string) (Schedule, bool) {
	s.RLock()
	defer s.RUnlock()
	value, found := s.data[key]
	return value, found
}

func (s *ScheduleMap) Write(key string, value Schedule) {
	s.Lock()
	defer s.Unlock()
	s.data[key] = value
}

func (s *ScheduleMap) Delete(key string) {
	s.Lock()
	defer s.Unlock()
	delete(s.data, key)
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdbackupschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdbackupschedules/status,verbs=get;update;patch

func (r *EtcdBackupScheduleReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	log := r.Log.WithValues("etcdbackupschedule", req.NamespacedName)

	resource := &etcdv1alpha1.EtcdBackupSchedule{}
	if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Add a finalizer if one does not exist.
	finalizerIsSet := false
	for _, f := range resource.ObjectMeta.Finalizers {
		if f == scheduleCancelFinalizer {
			finalizerIsSet = true
			break
		}
	}
	if !finalizerIsSet {
		resource.ObjectMeta.Finalizers = append(resource.ObjectMeta.Finalizers, scheduleCancelFinalizer)
		if err := r.Update(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	schedule, found := r.Schedules.Read(string(resource.UID))
	if !found {
		id, err := r.CronHandler.AddFunc(resource.Spec.Schedule, func() {
			log.Info("Creating EtcdBackup resource")
			err := r.fire(req.NamespacedName)
			if err != nil {
				log.Error(err, "Backup resource creation failed")
			}
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		r.Schedules.Write(string(resource.UID), Schedule{
			cronEntry: id,
		})
		return ctrl.Result{}, nil
	}

	// If the object is being deleted, clean it up from the cron pool.
	if !resource.ObjectMeta.DeletionTimestamp.IsZero() {
		r.CronHandler.Remove(schedule.cronEntry)
		r.Schedules.Delete(string(resource.UID))

		// Remove the finalizer.
		var newFinalizers []string
		for _, f := range resource.ObjectMeta.Finalizers {
			if f != scheduleCancelFinalizer {
				newFinalizers = append(newFinalizers, f)
			}
		}
		resource.ObjectMeta.Finalizers = newFinalizers
		if err := r.Update(ctx, resource); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *EtcdBackupScheduleReconciler) fire(resourceNamespacedName types.NamespacedName) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	resource := &etcdv1alpha1.EtcdBackupSchedule{}
	if err := r.Get(ctx, resourceNamespacedName, resource); err != nil {
		return client.IgnoreNotFound(err)
	}

	if err := r.Client.Create(ctx, &etcdv1alpha1.EtcdBackup{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: resource.ObjectMeta.Name + "-",
			Namespace:    resource.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(resource, etcdv1alpha1.GroupVersion.WithKind("EtcdBackupSchedule")),
			},
			Labels: map[string]string{
				scheduleLabel: resource.ObjectMeta.Name,
			},
		},
		Spec: resource.Spec.BackupTemplate,
	}); err != nil {
		return err
	}

	return nil
}

func (r *EtcdBackupScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdBackupSchedule{}).
		Complete(r)
}
