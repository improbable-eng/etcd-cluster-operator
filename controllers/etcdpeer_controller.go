package controllers

import (
	"context"
	"fmt"
	"net/url"
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
	etcdImage                     = "quay.io/coreos/etcd:v3.2.27"
	etcdAdvertiseClientURLsEnvVar = "ETCD_ADVERTISE_CLIENT_URLS"
	etcdInitialClusterEnvVar      = "ETCD_INITIAL_CLUSTER"
	etcdNameEnvVar                = "ETCD_NAME"
	etcdScheme                    = "http"
	etcdPeerPort                  = 2380
	appName                       = "etcd"
	appLabel                      = "app.kubernetes.io/app"
	clusterLabel                  = "etcd.improbable.io/cluster-name"
	peerLabel                     = "etcd.improbable.io/peer-name"
)

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicaset,verbs=get;update;patch;create

func initialMemberURL(member etcdv1alpha1.InitialClusterMember) *url.URL {
	return &url.URL{
		Scheme: etcdScheme,
		Host:   fmt.Sprintf("%s:%d", member.Host, etcdPeerPort),
	}
}

// staticBootstrapInitialCluster returns the value of `ETCD_INITIAL_CLUSTER`
// environment variable.
func staticBootstrapInitialCluster(static etcdv1alpha1.StaticBootstrap) string {
	s := make([]string, len(static.InitialCluster))
	// Put our peers in as the other entries
	for i, member := range static.InitialCluster {
		s[i] = fmt.Sprintf("%s=%s",
			member.Name,
			initialMemberURL(member).String())
	}
	return strings.Join(s, ",")
}

// advertiseURL builds the canonical URL of this peer from it's name and the
// cluster name.
func advertiseURL(etcdPeer etcdv1alpha1.EtcdPeer) *url.URL {
	return &url.URL{
		Scheme: "http",
		Host: fmt.Sprintf(
			"%s.%s.%s.svc:2380",
			etcdPeer.Name,
			etcdPeer.Namespace,
			etcdPeer.Spec.ClusterName,
		),
	}
}

func defineReplicaSet(peer etcdv1alpha1.EtcdPeer) appsv1.ReplicaSet {
	var replicas int32 = 1

	// We use the same labels for the replica set itself, the selector on
	// the replica set, and the pod template under the replica set.
	labels := map[string]string{
		appLabel:     appName,
		clusterLabel: peer.Spec.ClusterName,
		peerLabel:    peer.Name,
	}

	return appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Annotations:     make(map[string]string),
			Name:            peer.Name,
			Namespace:       peer.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(&peer, etcdv1alpha1.GroupVersion.WithKind("EtcdPeer"))},
		},
		Spec: appsv1.ReplicaSetSpec{
			// This will *always* be 1. Other peers are handled by other EtcdPeers.
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: make(map[string]string),
					Name:        peer.Name,
					Namespace:   peer.Namespace,
				},
				Spec: corev1.PodSpec{
					Hostname:  peer.Name,
					Subdomain: peer.Spec.ClusterName,
					Containers: []corev1.Container{
						{
							Name:  appName,
							Image: etcdImage,
							Env: []corev1.EnvVar{
								{
									Name:  etcdInitialClusterEnvVar,
									Value: staticBootstrapInitialCluster(peer.Spec.Bootstrap.Static),
								},
								{
									Name:  etcdNameEnvVar,
									Value: peer.Name,
								},
								{
									Name:  etcdAdvertiseClientURLsEnvVar,
									Value: advertiseURL(peer).String(),
								},
							},
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

	var peer etcdv1alpha1.EtcdPeer
	if err := r.Get(ctx, req.NamespacedName, &peer); err != nil {
		log.Error(err, "unable to fetch EtcdPeer")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.V(2).Info("Found EtcdPeer", "name", peer.Name)

	var existingReplicaSet appsv1.ReplicaSet
	err := r.Get(
		ctx,
		client.ObjectKey{
			Namespace: peer.Namespace,
			Name:      peer.Name,
		},
		&existingReplicaSet,
	)

	if apierrs.IsNotFound(err) {
		log.V(1).Info("Replica set does not exist, creating")
		replicaSet := defineReplicaSet(peer)

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
