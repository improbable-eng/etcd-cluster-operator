package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

// EtcdPeerReconciler reconciles a EtcdPeer object
type EtcdPeerReconciler struct {
	client.Client
	Log logr.Logger
}

const (
	etcdImage = "quay.io/coreos/etcd:v3.2.27"
)

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicaset,verbs=get;update;patch;create

func clusterDiscoveryString(bootstrapPeers []etcdv1alpha1.BootstrapPeer) string {
	s := make([]string, len(bootstrapPeers))
	for i, bp := range bootstrapPeers {
		s[i] = fmt.Sprintf("%s=http://%s:2380", bp.Name, bp.Host)
	}
	return strings.Join(s, ",")
}

func defineReplicaSet(etcdPeer etcdv1alpha1.EtcdPeer) appsv1.ReplicaSet {
	var replicas int32 = 1

	return appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          make(map[string]string),
			Annotations:     make(map[string]string),
			Name:            etcdPeer.Name,
			Namespace:       etcdPeer.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(&etcdPeer, etcdv1alpha1.GroupVersion.WithKind("EtcdPeer"))},
		},
		Spec: appsv1.ReplicaSetSpec{
			// This will *always* be 1. Other peers are handled by other EtcdPeers.
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "etcd",
					// Using the EtcdPeer's name as a label limits what the name can be
					"peer": etcdPeer.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "etcd",
						// Using the EtcdPeer's name as a label limits what the name can be
						"peer": etcdPeer.Name,
					},
					Annotations: make(map[string]string),
					Name:        etcdPeer.Name,
					Namespace:   etcdPeer.Namespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "etcd",
							Image: etcdImage,
						},
					},
				},
			},
		},
	}
}

func (r *EtcdPeerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("etcdpeer", req.NamespacedName)

	var etcdPeer etcdv1alpha1.EtcdPeer
	if err := r.Get(ctx, req.NamespacedName, &etcdPeer); err != nil {
		log.Error(err, "unable to fetch EtcdPeer")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.V(2).Info("Found EtcdPeer", "name", etcdPeer.Name)

	var existingReplicaSet appsv1.ReplicaSet
	err := r.Get(
		ctx,
		client.ObjectKey{
			Namespace: etcdPeer.Namespace,
			Name:      etcdPeer.Name,
		},
		&existingReplicaSet,
	)

	if apierrs.IsNotFound(err) {
		log.V(1).Info("Replica set does not exist, creating")
		replicaSet := defineReplicaSet(etcdPeer)

		if err := r.Create(ctx, &replicaSet); err != nil {
			log.Error(err, "unable to create ReplicaSet for EtcdPeer", "replicaSet", replicaSet)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check for some other error from the previous `r.Get`
	if err != nil {
		log.Error(err, "unable to query for replica sets")
		return ctrl.Result{}, err
	}

	log.V(2).Info("Replica set already exists")

	// TODO Additional logic here

	return ctrl.Result{}, nil
}

func (r *EtcdPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdPeer{}).
		// Watch for changes to ReplicaSet resources that an EtcdPeer owns.
		Owns(&appsv1.ReplicaSet{}).
		Complete(r)
}
