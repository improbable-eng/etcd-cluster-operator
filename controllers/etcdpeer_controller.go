package controllers

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
)

// EtcdPeerReconciler reconciles a EtcdPeer object
type EtcdPeerReconciler struct {
	client.Client
	Log logr.Logger
}

func ignoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicaset,verbs=get;update;patch;create

func (r *EtcdPeerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("etcdpeer", req.NamespacedName)

	var etcdPeer etcdv1alpha1.EtcdPeer
	if err := r.Get(ctx, req.NamespacedName, &etcdPeer); err != nil {
		log.Error(err, "unable to fetch EtcdPeer")
		return ctrl.Result{}, ignoreNotFound(err)
	}

	log.V(1).Info("Found EtcdPeer",
		"name", etcdPeer.Name,
		"bootstrapPeers", etcdPeer.Spec.BootstrapPeers)

	return ctrl.Result{}, nil
}

func (r *EtcdPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdPeer{}).
		Complete(r)
}
