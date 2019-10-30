package controllers

import (
	"context"
	"fmt"
	"github.com/improbable-eng/etcd-cluster-operator/internal/event"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"net/url"
	"time"

	"github.com/go-logr/logr"
	etcdclient "go.etcd.io/etcd/client"
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
	Log      logr.Logger
	Recorder record.EventRecorder
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

// updateStatus updates the EtcdCluster resource's status to be the current value of the cluster.
func (r *EtcdClusterReconciler) updateStatus(
	ctx context.Context,
	name types.NamespacedName,
	cluster *etcdv1alpha1.EtcdCluster,
	members *[]etcdclient.Member,
	reconcilerEvent event.EtcdClusterReconcilerEvent) error {

	log := r.Log.WithValues("cluster", name)

	if members != nil {
		cluster.Status.Members = make([]etcdv1alpha1.EtcdMember, len(*members))
		for i, member := range *members {
			cluster.Status.Members[i] = etcdv1alpha1.EtcdMember{
				Name: member.Name,
				ID:   member.ID,
			}
		}

		if err := r.Client.Status().Update(ctx, cluster); err != nil {
			log.Error(err, "Unable to update status field on EtcdCluster")
			return err
		}
	}
	return nil
}

func (r *EtcdClusterReconciler) reconcile(
	ctx context.Context,
	name types.NamespacedName,
	members *[]etcdclient.Member,
	cluster *etcdv1alpha1.EtcdCluster,
) (
	ctrl.Result,
	event.EtcdClusterReconcilerEvent,
	error) {

	log := r.Log.WithValues("cluster", name)

	// Apply defaults in case a defaulting webhook has not been deployed.
	cluster.Default()

	// Validate in case a validating webhook has not been deployed
	err := cluster.ValidateCreate()
	if err != nil {
		log.Error(err, "invalid EtcdCluster")
		return ctrl.Result{}, nil, nil
	}

	service := &v1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch EtcdCluster service")
			return ctrl.Result{}, nil, err
		}
		service = headlessServiceForCluster(cluster)
		if err := r.Create(ctx, service); err != nil {
			log.Error(err, "unable to create Service", "service", service.Name)
			return ctrl.Result{}, nil, err
		}
		log.V(1).Info("Created Service", "service", service.Name)
		return ctrl.Result{}, &event.ServiceCreatedEvent{Object: cluster, ServiceName: service.Name}, nil
	}
	log.V(2).Info("Service exists", "service", service.Name)

	if members == nil {
		peers := &etcdv1alpha1.EtcdPeerList{}
		if err := r.List(ctx, peers, client.MatchingFields{clusterNameSpecField: cluster.Name}); err != nil {
			log.Error(err, "unable to list peers")
			return ctrl.Result{}, nil, err
		}

		if cluster.Spec.Replicas != nil && int32(len(peers.Items)) < *cluster.Spec.Replicas {
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
				return ctrl.Result{}, nil, err
			}
			return ctrl.Result{}, &event.PeerCreatedEvent{Object: cluster, PeerName: peer.Name}, nil
		}
	} else {
		log.V(2).Info("Cluster communication established")
	}
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil, nil
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list

func (r *EtcdClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("cluster", req.NamespacedName)

	cluster := &etcdv1alpha1.EtcdCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		log.Error(err, "unable to fetch EtcdCluster resource")
		return ctrl.Result{}, client.IgnoreNotFound(err)
		// special 'exit early' case.
	}

	// Attempt to dial the etcd cluster, recording the cluster response if we can
	members := &[]etcdclient.Member{}
	if memberSlice, err := r.connectToEtcd(ctx, cluster); err != nil {
		log.Error(err, "Unable to contact etcd cluster")
		members = nil
	} else {
		members = &memberSlice
	}

	// Perform a reconcile, getting back the desired result, any errors, and a clusterEvent. This is an internal concept
	// and is not the same as the Kubernetes event, although it is used to make one later.
	result, clusterEvent, err := r.reconcile(ctx, req.NamespacedName, members, cluster)
	if err != nil {
		log.Error(err, "Failed to reconcile")
	}

	// The update status takes in the cluster definition, and the member list from etcd as of *before we ran reconcile*.
	// We also get the event, which may contain rich information about what we did (such as the new member name on a
	// MemberAdded event).
	err = r.updateStatus(ctx, req.NamespacedName, cluster, members, clusterEvent)
	if err != nil {
		log.Error(err, "Failed to update status")
	}

	// Finally, the event is used to generate a Kubernetes event by calling `Record` and passing in the recorder.
	if clusterEvent != nil {
		clusterEvent.Record(r.Recorder)
	}

	return result, nil
}

func (r *EtcdClusterReconciler) connectToEtcd(ctx context.Context, cluster *etcdv1alpha1.EtcdCluster) ([]etcdclient.Member, error) {
	etcdConfig := etcdClientConfig(cluster)
	c, err := etcdclient.New(*etcdConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to connect to etcd")
	}

	membersAPI := etcdclient.NewMembersAPI(c)
	members, err := membersAPI.List(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to list members of etcd cluster")
	}

	return members, nil
}

func etcdClientConfig(cluster *etcdv1alpha1.EtcdCluster) *etcdclient.Config {
	peerNames := expectedPeerNamesForCluster(cluster)
	peerAddresses := make([]string, len(peerNames))
	for i, peerName := range peerNames {
		u := &url.URL{
			Scheme: etcdScheme,
			Host:   fmt.Sprintf("%s:%d", expectedURLForPeer(cluster, peerName), etcdClientPort),
		}
		peerAddresses[i] = u.String()
	}
	return &etcdclient.Config{
		Endpoints: peerAddresses,
		Transport: etcdclient.DefaultTransport,
	}
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
			Storage: cluster.Spec.Storage.DeepCopy(),
		},
	}
}

func nthPeerName(cluster *etcdv1alpha1.EtcdCluster, i int) string {
	return fmt.Sprintf("%s-%d", cluster.Name, i)
}

func initialClusterMembers(cluster *etcdv1alpha1.EtcdCluster) []etcdv1alpha1.InitialClusterMember {
	names := expectedPeerNamesForCluster(cluster)
	members := make([]etcdv1alpha1.InitialClusterMember, len(names))
	for i := range members {
		members[i] = etcdv1alpha1.InitialClusterMember{
			Name: names[i],
			Host: expectedURLForPeer(cluster, names[i]),
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
	for i := range names {
		names[i] = nthPeerName(cluster, i)
	}
	return names
}

// expectedURLForPeer returns the Kubernetes-internal DNS name that the given peer can be contacted on, from any
// namespace. This does not include a port or scheme, so can be used as either a "peer" URL using the peer port or the
// client URL using the client port.
func expectedURLForPeer(cluster *etcdv1alpha1.EtcdCluster, peerName string) string {
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
