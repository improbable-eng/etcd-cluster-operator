package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/controllers/peer"
)

// EtcdPeerReconciler reconciles a EtcdPeer object
type EtcdPeerReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=list;get;create;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=list;get;create;watch;delete

func hasPvcDeletionFinalizer(peer *etcdv1alpha1.EtcdPeer) bool {
	return sets.NewString(peer.ObjectMeta.Finalizers...).Has(etcdv1alpha1.PVCCleanupFinalizer)
}

func (r *EtcdPeerReconciler) nextAction(log logr.Logger, state *peer.State) Action {
	if state.Peer == nil {
		log.Info("EtcdPeer not found")
		return nil
	}

	// Validate in case a validating webhook has not been deployed
	if err := state.Peer.ValidateCreate(); err != nil {
		log.Error(err, "invalid EtcdPeer")
		return nil
	}

	var action Action
	switch {
	case !state.Peer.ObjectMeta.DeletionTimestamp.IsZero() && hasPvcDeletionFinalizer(state.Peer):
		// Peer deleted and requires PVC cleanup
		action = &peer.PVCDeleter{Log: log, Client: r.Client, Peer: state.Peer}

	case !state.Peer.ObjectMeta.DeletionTimestamp.IsZero():
		// Peer deleted, no PVC cleanup

	case state.PVC == nil:
		// Create PVC
		action = &CreateRuntimeObject{log: log, client: r.Client, obj: state.DesiredPVC}

	case state.ReplicaSet == nil:
		// Create Replicaset
		action = &CreateRuntimeObject{log: log, client: r.Client, obj: state.DesiredReplicaSet}
	}
	return action
}

func (r *EtcdPeerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log := r.Log.WithValues("peer", req.NamespacedName)

	state, err := peer.GetState(log, r.Client, ctx, req)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error while getting current state: %s", err)
	}

	action := r.nextAction(log, state)
	if action != nil {
		return ctrl.Result{}, action.Execute(ctx)
	}

	return ctrl.Result{}, nil
}

type pvcMapper struct{}

var _ handler.Mapper = &pvcMapper{}

// Map looks up the peer name label from the PVC and generates a reconcile
// request for *that* name in the namespace of the pvc.
// This mapper ensures that we only wake up the Reconcile function for changes
// to PVCs related to EtcdPeer resources.
// PVCs are deliberately not owned by the peer, to ensure that they are not
// garbage collected along with the peer.
// So we can't use OwnerReference handler here.
func (m *pvcMapper) Map(o handler.MapObject) []reconcile.Request {
	requests := []reconcile.Request{}
	labels := o.Meta.GetLabels()
	if peerName, found := labels[etcdv1alpha1.PeerLabel]; found {
		requests = append(
			requests,
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      peerName,
					Namespace: o.Meta.GetNamespace(),
				},
			},
		)
	}
	return requests
}

func (r *EtcdPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdPeer{}).
		// Watch for changes to ReplicaSet resources that an EtcdPeer owns.
		Owns(&appsv1.ReplicaSet{}).
		// We can use a simple EnqueueRequestForObject handler here as the PVC
		// has the same name as the EtcdPeer resource that needs to be enqueued
		Watches(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: &pvcMapper{},
		}).
		Complete(r)
}
