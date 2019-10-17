package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

const (
	clusterNameSpecField = "spec.clusterName"
)

// EtcdClusterReconciler reconciles a EtcdCluster object
type EtcdClusterReconciler struct {
	client.Client
	Log logr.Logger
}

func headlessServiceForCluster(cluster *etcdv1alpha1.EtcdCluster) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cluster, etcdv1alpha1.GroupVersion.WithKind("EtcdCluster")),
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
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;create

func (r *EtcdClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("cluster", req.NamespacedName)

	cluster := &etcdv1alpha1.EtcdCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		log.Error(err, "unable to fetch EtcdCluster")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	service := &v1.Service{}
	if err := r.Get(ctx, req.NamespacedName, service); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch EtcdCluster service")
			return ctrl.Result{}, err
		}
		service = headlessServiceForCluster(cluster)
		if err := r.Create(ctx, service); err != nil {
			log.Error(err, "unable to create Service", "service", service.Name)
			return ctrl.Result{}, err
		}
		log.V(1).Info("Created Service", "service", service.Name)
		return ctrl.Result{}, nil
	}
	log.V(2).Info("Service exists", "service", service.Name)

	peers := &etcdv1alpha1.EtcdPeerList{}
	if err := r.List(ctx, peers, client.MatchingFields{clusterNameSpecField: cluster.Name}); err != nil {
		log.Error(err, "unable to list peers")
		return ctrl.Result{}, err
	}

	// Assume we can't communicate with the cluster
	log.V(1).Info("Unable to communicate with etcd cluster")
	if int32(len(peers.Items)) < *cluster.Spec.Replicas {

		// Create more peers
		peerName := nextAvailablePeerName(cluster, peers.Items)
		log.V(1).Info("Insufficient peers for replicas, adding new peer",
			"current-peers", len(peers.Items),
			"desired-peers", cluster.Spec.Replicas,
			"peer", peerName)
		peer := peerForCluster(cluster, peerName)
		if err := r.Create(ctx, peer); err != nil {
			log.Error(err, "Failed to create peer",
				"peer", peerName)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func peerForCluster(cluster *etcdv1alpha1.EtcdCluster, peerName string) *etcdv1alpha1.EtcdPeer {
	return &etcdv1alpha1.EtcdPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peerName,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cluster, etcdv1alpha1.GroupVersion.WithKind("EtcdCluster")),
			},
			Labels: map[string]string{
				appLabel:     appName,
				clusterLabel: cluster.Name,
			},
		},
		Spec: etcdv1alpha1.EtcdPeerSpec{
			ClusterName: cluster.Name,
			Bootstrap: &etcdv1alpha1.Bootstrap{
				Static: &etcdv1alpha1.StaticBootstrap{
					InitialCluster: initialClusterMembers(cluster),
				},
			},
		},
	}
}

func nthPeerName(cluster *etcdv1alpha1.EtcdCluster, i int) string {
	return fmt.Sprintf("%s-%d", cluster.Name, i)
}

func initialClusterMembers(cluster *etcdv1alpha1.EtcdCluster) []etcdv1alpha1.InitialClusterMember {
	names := expectedPeerNamesForCluster(cluster)
	members := make([]etcdv1alpha1.InitialClusterMember, len(names))
	for i, _ := range members {
		members[i] = etcdv1alpha1.InitialClusterMember{
			Name: names[i],
			Host: expectedAdvertisePeerURLForPeer(cluster, names[i]),
		}
	}
	return members
}

func nextAvailablePeerName(cluster *etcdv1alpha1.EtcdCluster, peers []etcdv1alpha1.EtcdPeer) string {
	for i := 0; ; i++ {
		candidateName := nthPeerName(cluster, i)
		nameClash := false
		for _, peer := range peers {
			if peer.Name == candidateName {
				nameClash = true
				break
			}
		}
		if !nameClash {
			return candidateName
		}
	}
}

func expectedPeerNamesForCluster(cluster *etcdv1alpha1.EtcdCluster) (names []string) {
	names = make([]string, int(*cluster.Spec.Replicas))
	for i, _ := range names {
		names[i] = nthPeerName(cluster, i)
	}
	return names
}

func expectedAdvertisePeerURLForPeer(cluster *etcdv1alpha1.EtcdCluster, peerName string) string {
	return fmt.Sprintf("%s.%s.%s.svc",
		peerName,
		cluster.Name,
		cluster.Namespace,
	)
}

func (r *EtcdClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(&etcdv1alpha1.EtcdPeer{},
		clusterNameSpecField,
		func(obj runtime.Object) []string {
			peer, ok := obj.(*etcdv1alpha1.EtcdPeer)
			if !ok {
				// Fail? We've been asked to index the cluster name for something that isn't a peer.
				return nil
			}
			return []string{peer.Spec.ClusterName}
		}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdCluster{}).
		Owns(&v1.Service{}).
		Owns(&etcdv1alpha1.EtcdPeer{}).
		Complete(r)
}
