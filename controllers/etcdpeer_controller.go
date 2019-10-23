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
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcdenvvar"
)

// EtcdPeerReconciler reconciles a EtcdPeer object
type EtcdPeerReconciler struct {
	client.Client
	Log logr.Logger
}

const (
	etcdImage  = "quay.io/coreos/etcd:v3.2.27"
	etcdScheme = "http"
	peerLabel  = "etcd.improbable.io/peer-name"
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
func advertiseURL(etcdPeer etcdv1alpha1.EtcdPeer, port int32) *url.URL {
	return &url.URL{
		Scheme: etcdScheme,
		Host: fmt.Sprintf(
			"%s.%s.%s.svc:%d",
			etcdPeer.Name,
			etcdPeer.Spec.ClusterName,
			etcdPeer.Namespace,
			port,
		),
	}
}

func bindAllAddress(port int) *url.URL {
	return &url.URL{
		Scheme: etcdScheme,
		Host:   fmt.Sprintf("0.0.0.0:%d", port),
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
									Name:  etcdenvvar.InitialCluster,
									Value: staticBootstrapInitialCluster(*peer.Spec.Bootstrap.Static),
								},
								{
									Name:  etcdenvvar.Name,
									Value: peer.Name,
								},
								{
									Name:  etcdenvvar.InitialAdvertisePeerURLs,
									Value: advertiseURL(peer, etcdPeerPort).String(),
								},
								{
									Name:  etcdenvvar.AdvertiseClientURLs,
									Value: advertiseURL(peer, etcdClientPort).String(),
								},
								{
									Name:  etcdenvvar.ListenPeerURLs,
									Value: bindAllAddress(etcdPeerPort).String(),
								},
								{
									Name:  etcdenvvar.ListenClientURLs,
									Value: bindAllAddress(etcdClientPort).String(),
								},
								{
									Name:  etcdenvvar.InitialClusterState,
									Value: "new",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "etcd-client",
									ContainerPort: etcdClientPort,
								},
								{
									Name:          "etcd-peer",
									ContainerPort: etcdPeerPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "etcd-data",
									MountPath: etcdDataMountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "etcd-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: peer.Name,
								},
							},
						},
					},
				},
			},
		},
	}
}

func pvcForPeer(peer *etcdv1alpha1.EtcdPeer) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peer.Name,
			Namespace: peer.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(peer, etcdv1alpha1.GroupVersion.WithKind("EtcdPeer")),
			},
		},
		Spec: *peer.Spec.Storage.VolumeClaimTemplate.DeepCopy(),
	}
}

func (r *EtcdPeerReconciler) maybeCreatePvc(ctx context.Context, peer *etcdv1alpha1.EtcdPeer) (created bool, err error) {
	objectKey := client.ObjectKey{
		Name:      peer.Name,
		Namespace: peer.Namespace,
	}
	// Check for existing object
	pvc := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, objectKey, pvc)
	// Object exists
	if err == nil {
		return false, nil
	}
	// Error when fetching the object
	if !apierrs.IsNotFound(err) {
		return false, err
	}
	// Object does not exist
	err = r.Create(ctx, pvcForPeer(peer))
	if apierrs.IsAlreadyExists(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
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

	// Apply defaults in case a defaulting webhook has not been deployed.
	peer.Default()

	created, err := r.maybeCreatePvc(ctx, &peer)
	if err != nil || created {
		return ctrl.Result{}, err
	}

	var existingReplicaSet appsv1.ReplicaSet
	err = r.Get(
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
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}
