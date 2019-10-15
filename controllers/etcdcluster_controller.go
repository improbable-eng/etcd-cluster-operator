package controllers

import (
	"context"
	v1 "k8s.io/api/core/v1"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

// EtcdClusterReconciler reconciles a EtcdCluster object
type EtcdClusterReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters/status,verbs=get;update;patch

func (r *EtcdClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("etcdcluster", req.NamespacedName)

	var cluster etcdv1alpha1.EtcdCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		log.Error(err, "unable to fetch EtcdCluster")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// your logic here
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(&cluster, etcdv1alpha1.GroupVersion.WithKind("EtcdCluster")),
			},
			Labels: map[string]string{
				appLabel:     appName,
				clusterLabel: cluster.Name,
			},
		},
		Spec: v1.ServiceSpec{
			ClusterIP:                v1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Selector: map[string]string{
				appLabel:     appName,
				clusterLabel: cluster.Name,
			},
			Ports: []v1.ServicePort{
				{
					Name:     "etcd-client",
					Protocol: "TCP",
					Port:     etcdClientPort,
				},
				{
					Name:     "etcd-peer",
					Protocol: "TCP",
					Port:     etcdPeerPort,
				},
			},
		},
	}
	if err := r.Create(ctx, service); err != nil {
		log.Error(err, "unable to create Service")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *EtcdClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdCluster{}).
		Complete(r)
}
