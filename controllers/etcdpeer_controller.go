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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

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

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=list;get;create;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=list;get;create;watch

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

func clusterStateValue(cs etcdv1alpha1.InitialClusterState) string {
	if cs == etcdv1alpha1.InitialClusterStateNew {
		return "new"
	} else if cs == etcdv1alpha1.InitialClusterStateExisting {
		return "existing"
	} else {
		return ""
	}
}

func defineReplicaSet(peer etcdv1alpha1.EtcdPeer, log logr.Logger) appsv1.ReplicaSet {
	var replicas int32 = 1

	// We use the same labels for the replica set itself, the selector on
	// the replica set, and the pod template under the replica set.
	labels := map[string]string{
		appLabel:     appName,
		clusterLabel: peer.Spec.ClusterName,
		peerLabel:    peer.Name,
	}

	replicaSet := appsv1.ReplicaSet{
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
									Name:  etcdenvvar.InitialClusterToken,
									Value: peer.Spec.ClusterName,
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
									Value: clusterStateValue(peer.Spec.Bootstrap.InitialClusterState),
								},
								{
									Name:  etcdenvvar.DataDir,
									Value: etcdDataMountPath,
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

	if peer.Spec.PodTemplate != nil {
		if peer.Spec.PodTemplate.Metadata != nil {
			// Stamp annotations
			for name, value := range peer.Spec.PodTemplate.Metadata.Annotations {
				if !etcdv1alpha1.IsInvalidUserProvidedAnnotationName(name) {
					if _, found := replicaSet.Spec.Template.Annotations[name]; !found {
						replicaSet.Spec.Template.Annotations[name] = value
					} else {
						// This will only check against an annotation that we set ourselves.
						log.V(2).Info("Ignoring annotation, we already have one with that name",
							"annotation-name", name)
					}
				} else {
					// In theory, this code is unreachable as we check this validation at the start of the reconcile
					// loop. See https://xkcd.com/2200
					log.V(2).Info("Ignoring annotation, applying etcd.improbable.io/ annotations is not supported",
						"annotation-name", name)
				}
			}
		}
	}

	return replicaSet
}

func pvcForPeer(peer *etcdv1alpha1.EtcdPeer) *corev1.PersistentVolumeClaim {
	labels := map[string]string{
		appLabel:     appName,
		clusterLabel: peer.Spec.ClusterName,
		peerLabel:    peer.Name,
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peer.Name,
			Namespace: peer.Namespace,
			Labels:    labels,
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
	// Maybe a stale cache.
	if apierrs.IsAlreadyExists(err) {
		return false, fmt.Errorf("stale cache error: object was not found in cache but creation failed with AlreadyExists error: %w", err)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *EtcdPeerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("peer", req.NamespacedName)

	var peer etcdv1alpha1.EtcdPeer
	if err := r.Get(ctx, req.NamespacedName, &peer); err != nil {
		log.Error(err, "unable to fetch EtcdPeer")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.V(2).Info("Found EtcdPeer resource")

	// Apply defaults in case a defaulting webhook has not been deployed.
	peer.Default()

	// Validate in case a validating webhook has not been deployed
	err := peer.ValidateCreate()
	if err != nil {
		log.Error(err, "invalid EtcdPeer")
		return ctrl.Result{}, nil
	}

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
		replicaSet := defineReplicaSet(peer, log)
		log.V(1).Info("Replica set does not exist, creating",
			"replica-set", replicaSet.Name)
		if err := r.Create(ctx, &replicaSet); err != nil {
			log.Error(err, "unable to create ReplicaSet for EtcdPeer", "replica-set", replicaSet)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check for some other error from the previous `r.Get`
	if err != nil {
		log.Error(err, "unable to query for replica sets")
		return ctrl.Result{}, err
	}

	log.V(2).Info("Replica set already exists", "replica-set", existingReplicaSet.Name)

	// TODO Additional logic here

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
	if peerName, found := labels[peerLabel]; found {
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
