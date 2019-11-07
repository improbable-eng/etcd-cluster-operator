package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	etcdclient "go.etcd.io/etcd/client"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/reconcilerevent"
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
func (r *EtcdClusterReconciler) updateStatus(ctx context.Context,
	cluster *etcdv1alpha1.EtcdCluster,
	members *[]etcdclient.Member,
	peers *etcdv1alpha1.EtcdPeerList,
	reconcilerEvent reconcilerevent.ReconcilerEvent) error {

	if members != nil {
		cluster.Status.Members = make([]etcdv1alpha1.EtcdMember, len(*members))
		for i, member := range *members {
			cluster.Status.Members[i] = etcdv1alpha1.EtcdMember{
				Name: member.Name,
				ID:   member.ID,
			}
		}
	} else {
		if cluster.Status.Members == nil {
			cluster.Status.Members = make([]etcdv1alpha1.EtcdMember, 0)
		} else {
			// In this case we don't have up-to date members for whatever reason, which could be a temporary disruption
			// such as a network issue. But we also know the status on the resource already has a member list. So just
			// leave that (stale) data where it is and don't change it.
		}
	}

	cluster.Status.Replicas = int32(len(peers.Items))

	if err := r.Client.Status().Update(ctx, cluster); err != nil {
		return err
	}

	return nil
}

// reconcile (little r) does the 'dirty work' of deciding if we wish to perform an action upon the Kubernetes cluster to
// match our desired state. We will always perform exactly one or zero actions. The intent is that when multiple actions
// are required we will do only one, then requeue this function to perform the next one. This helps to keep the code
// simple, as every 'go around' only performs a single change.
//
// We strictly use the term "member" to refer to an item in the etcd membership list, and "peer" to refer to an
// EtcdPeer resource created by us that ultimately creates a pod running the etcd binary.
//
// In order to allow other parts of this operator to know what we've done, we return a `reconcilerevent.ReconcilerEvent`
// that should describe to the rest of this codebase what we have done.
func (r *EtcdClusterReconciler) reconcile(
	ctx context.Context,
	members *[]etcdclient.Member,
	peers *etcdv1alpha1.EtcdPeerList,
	cluster *etcdv1alpha1.EtcdCluster,
) (
	ctrl.Result,
	reconcilerevent.ReconcilerEvent,
	error,
) {
	name := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	log := r.Log.WithValues("cluster", name)

	// The first port of call is to verify that the service exists. This service is headless, and is designed to provide
	// the underlying etcd pods with a consistent network identity such that they can dial each other. It *can* be used
	// for client access to etcd if a user desires, but users are also encouraged to create their own services for their
	// client access if that is more convenient.
	service := &v1.Service{}
	if err := r.Get(ctx, name, service); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil, fmt.Errorf("unable to fetch EtcdCluster service: %w", err)
		}
		service = headlessServiceForCluster(cluster)
		if err := r.Create(ctx, service); err != nil {
			return ctrl.Result{}, nil, fmt.Errorf("unable to create service: %w", err)
		}
		log.V(1).Info("Created Service", "service", service.Name)
		return ctrl.Result{}, &reconcilerevent.ServiceCreatedEvent{Object: cluster, ServiceName: service.Name}, nil
	}
	log.V(2).Info("Service exists", "service", service.Name)

	// There are two big branches of behaviour: Either we (the operator) can contact the etcd cluster, or we can't. If
	// the `members` pointer is nil that means we were unsuccessful in our attempts to connect. Otherwise it indicates
	// a list of existing members.
	if members == nil {
		// We don't have communication with the cluster. This could be for a number of reasons, for example:
		//
		// * The cluster isn't up yet because we've not yet made the peers (during bootstrapping)
		// * Pod rescheduling due to node failure or administrator action has made the cluster non-quorate
		// * Network disruption inside the Kubernetes cluster has prevented us from connecting
		//
		// We treat the member list from etcd as a canonical list of what members should exist, so without it we're
		// unable to reconcile what peers we should create. Most of the cases listed above will be solved by outside
		// action. Either a network disruption will heal, or the scheduler will reschedule etcd pods. So no action is
		// required.
		//
		// Bootstrapping is a notable exception. We have to create our peer resources so that the pods will be created
		// in the first place. Which for us presents a 'chicken-and-egg' problem. Our solution is to take special
		// action during a bootstrapping situation. We can't accurately detect that the cluster has actually just been
		// bootstrapped in a reliable way (e.g., searching for the creation time on the cluster resource we're
		// reconciling). So our 'best effort' is to see if our desired peers is less than our actual peers. If so, we
		// just create new peers with bootstrap instructions.
		//
		// This may mean that we take a 'strange' action in some cases, such as a network disruption during a scale-up
		// event where we may add peers without first adding them to the etcd internal membership list (like we would
		// during normal operation). However, this situation will eventually heal:
		//
		// 1. We incorrectly create a blank peer
		// 2. The network split heals
		// 3. The cluster rejects the peer as it doesn't know about it
		// 4. The operator reconciles, and removes the peer that the membership list doesn't see
		// 5. We enter the normal scale-up behaviour, adding the peer to the membership list *first*.
		//
		// We are also careful to never *remove* a peer in this 'no contact' state. So while we may erroneously add
		// peers in some edge cases, we will never remove a peer without first confirming the action is correct with
		// the cluster.
		if cluster.Spec.Replicas != nil && int32(len(peers.Items)) < *cluster.Spec.Replicas {
			// We have fewer peers than our replicas, so create a new peer. We do this one at a time, so only make
			// *one* peer.
			peerName := nextAvailablePeerName(cluster, peers.Items)
			log.V(1).Info(
				"Insufficient peers for replicas and unable to contact etcd. Assuming we're bootstrapping adding new peer. (If we're not we'll heal this later.)",
				"current-peers", len(peers.Items),
				"desired-peers", cluster.Spec.Replicas,
				"peer", peerName)
			peer := peerForCluster(cluster, peerName)
			configurePeerBootstrap(peer, cluster)
			if err := r.Create(ctx, peer); err != nil {
				return ctrl.Result{}, nil, fmt.Errorf("failed to create EtcdPeer %s: %w", peerName, err)
			}
			return ctrl.Result{}, &reconcilerevent.PeerCreatedEvent{Object: cluster, PeerName: peer.Name}, nil
		} else {
			// We can't contact the cluster, and we don't *think* we need more peers. So it's unsafe to take any
			// action.
		}
	} else {
		// In this branch we *do* have communication with the cluster, and have retrieved a list of members. We treat
		// this list as canonical, and therefore reconciliation occurs in two cycles:
		//
		// 1. We reconcile the membership list to match our expected number of replicas (`spec.replicas`), adding
		//    members one at a time until reaching the correct number.
		// 2. We reconcile the number of peer resources to match the membership list.
		//
		// This 'two-tier' process may seem odd, but is designed to mirror etcd's operations guide for adding and
		// removing members. During a scale-up, etcd itself expects that new peers will be announced to it *first* using
		// the membership API, and then the etcd process started on the new machine. During a scale down the same is
		// true, where the membership API should be told that a member is to be removed before it is shut down.
		log.V(2).Info("Cluster communication established")

		// We reconcile the peer resources against the membership list before changing the membership list. This is the
		// opposite way around to what we just said. But is important. It means that if you perform a scale-up to five
		// from three, we'll add the fourth node to the membership list, then add the peer for the fourth node *before*
		// we consider adding the fifth node to the membership list.

		// Add EtcdPeer resources for members that do not have one.
		for _, member := range *members {
			// Use this instead of member.Name. If we've just added the peer for this member then the member might not
			// have that field set, so instead figure it out from the peerURL.
			expectedPeerName, err := peerNameForMember(member)
			if err != nil {
				log.Error(err, "Found member with no identifiable name, unable to reconcile it, skipping")
				continue
			}
			memberHasPeer := false
			for _, peer := range peers.Items {
				if expectedPeerName == peer.Name {
					memberHasPeer = true
					break
				}
			}
			if !memberHasPeer {
				// We have found a member without a peer. We should add an EtcdPeer resource for this member
				peerName, err := peerNameForMember(member)
				if err != nil {
					return ctrl.Result{}, nil, err
				}
				log.V(1).Info(
					"Found member in etcd's API that has no EtcdPeer resource representation, adding one.",
					"member-name", peerName)
				peer := peerForCluster(cluster, peerName)
				configureJoinExistingCluster(peer, cluster, *members)

				if err := r.Create(ctx, peer); err != nil {
					return ctrl.Result{}, nil, fmt.Errorf("failed to create EtcdPeer %s: %w", peer.Name, err)
				}
				return ctrl.Result{}, &reconcilerevent.PeerCreatedEvent{Object: cluster, PeerName: peer.Name}, nil
			}
		}

		// Remove EtcdPeer resources which do not have members.
		// TODO(#35) Implement scale-down

		// If we've reached this point we're sure that the EtcdPeer resources in the cluster match the contents of the
		// membership API. The next step is checking to see if we want to mutate the membership API of etcd to scale up
		// or down. However we don't want to do this unless the cluster has stabilised from a previous member addition.
		if isAllMembersStable(*members) {
			// It's finally time to see if we need to mutate the membership API to bring it in-line with our expected
			// replicas. There are three cases, we have enough members, too few, or too many. The order we check these
			// cases in is irrelevant as only one can possibly be true.
			if *cluster.Spec.Replicas > int32(len(*members)) {
				// There are too few members for the expected number of replicas. Add a new member.
				peerName := nextAvailablePeerName(cluster, peers.Items)
				peerURL := &url.URL{
					Scheme: etcdScheme,
					Host:   fmt.Sprintf("%s:%d", expectedURLForPeer(cluster, peerName), etcdPeerPort),
				}
				log.V(1).Info("Too few members for expected replica count, adding new member.",
					"expected-replicas", *cluster.Spec.Replicas,
					"actual-members", len(*members),
					"peer-name", peerName,
					"peer-url", peerURL)
				member, err := r.addEtcdMember(ctx, cluster, peerURL.String())
				if err != nil {
					return ctrl.Result{}, nil, fmt.Errorf("failed to add new member %s: %w", peerName, err)
				}
				// Requeue, as we don't watch on the membership list. So we don't auto-detect the change we just made.
				// TODO Implement custom watch on etcd membership API, and remove this requeue after member addition
				return ctrl.Result{RequeueAfter: time.Second * 10}, &reconcilerevent.MemberAddedEvent{Object: cluster, Member: member}, nil
			} else if *cluster.Spec.Replicas < int32(len(*members)) {
				// There are too many members for the expected number of replicas. Remove the member with the highest
				// ordinal
				// TODO Implement scale-down
			} else {
				// Exactly the correct number of members. Make no change, all is well with the world.
				log.V(2).Info("Expected number of replicas aligned with actual number.",
					"expected-replicas", *cluster.Spec.Replicas,
					"actual-members", len(*members))
			}
		}
	}
	// This is the fall-through 'all is right with the world' case. We requeue here with a ten second timeout. This is
	// so that we can recheck the membership API. We currently do not implement any watch on the etcd API, and as it is
	// not a Kubernetes API we will have to implement one custom.

	// TODO Implement custom watch on etcd membership API, and remove this regular requeue
	return ctrl.Result{RequeueAfter: time.Second * 10}, nil, nil
}

func isAllMembersStable(members []etcdclient.Member) bool {
	// We define 'stable' as 'has ever connected', which we detect by looking for the name on the member
	for _, member := range members {
		if member.Name == "" {
			// No name, means it's not up, so the cluster isn't stable.
			return false
		}
	}
	// Didn't find any members with no name, so stable.
	return true
}

func peerNameForMember(member etcdclient.Member) (string, error) {
	if member.Name != "" {
		return member.Name, nil
	} else if len(member.PeerURLs) > 0 {
		// Just take the first
		peerURL := member.PeerURLs[0]
		u, err := url.Parse(peerURL)
		if err != nil {
			return "", err
		}
		return strings.Split(u.Host, ".")[0], nil
	} else {
		return "", errors.New("nothing on member that could give a name")
	}
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *EtcdClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("cluster", req.NamespacedName)

	cluster := &etcdv1alpha1.EtcdCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
		// special 'exit early' case.
	}

	// Apply defaults in case a defaulting webhook has not been deployed.
	cluster.Default()

	// Validate in case a validating webhook has not been deployed
	err := cluster.ValidateCreate()
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid EtcdCluster resource: %w", err)
	}

	// Attempt to dial the etcd cluster, recording the cluster response if we can
	members := &[]etcdclient.Member{}
	if memberSlice, err := r.getEtcdMembers(ctx, cluster); err != nil {
		log.V(2).Info("Unable to contact etcd cluster", "error", err)
		members = nil
	} else {
		members = &memberSlice
	}

	// List peers
	peers := &etcdv1alpha1.EtcdPeerList{}
	if err := r.List(ctx, peers, client.MatchingFields{clusterNameSpecField: cluster.Name}); err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to list peers: %w", err)
	}

	// Perform a reconcile, getting back the desired result, any utilerrors, and a clusterEvent. This is an internal concept
	// and is not the same as the Kubernetes event, although it is used to make one later.
	result, clusterEvent, reconcileErr := r.reconcile(ctx, members, peers, cluster)
	if reconcileErr != nil {
		log.Error(reconcileErr, "Failed to reconcile")
	}

	// The update status takes in the cluster definition, and the member list from etcd as of *before we ran reconcile*.
	// We also get the event, which may contain rich information about what we did (such as the new member name on a
	// MemberAdded event).
	updateStatusErr := r.updateStatus(ctx, cluster, members, peers, clusterEvent)
	if updateStatusErr != nil {
		log.Error(updateStatusErr, "Failed to update status")
	}

	// Finally, the event is used to generate a Kubernetes event by calling `Record` and passing in the recorder.
	if clusterEvent != nil {
		clusterEvent.Record(r.Recorder)
	}

	return result, utilerrors.NewAggregate([]error{reconcileErr, updateStatusErr})
}

func (r *EtcdClusterReconciler) addEtcdMember(ctx context.Context, cluster *etcdv1alpha1.EtcdCluster, peerURL string) (*etcdclient.Member, error) {
	etcdConfig := etcdClientConfig(cluster)
	c, err := etcdclient.New(*etcdConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to etcd: %w", err)
	}

	membersAPI := etcdclient.NewMembersAPI(c)
	return membersAPI.Add(ctx, peerURL)
}

func (r *EtcdClusterReconciler) getEtcdMembers(ctx context.Context, cluster *etcdv1alpha1.EtcdCluster) ([]etcdclient.Member, error) {
	etcdConfig := etcdClientConfig(cluster)
	c, err := etcdclient.New(*etcdConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to etcd: %w", err)
	}

	membersAPI := etcdclient.NewMembersAPI(c)
	members, err := membersAPI.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to list members of etcd cluster: %w", err)
	}

	return members, nil
}

func etcdClientConfig(cluster *etcdv1alpha1.EtcdCluster) *etcdclient.Config {
	serviceURL := &url.URL{
		Scheme: etcdScheme,
		// We (the operator) are quite probably in a different namespace to the cluster, so we need to use a fully
		// defined URL.
		Host: fmt.Sprintf("%s.%s.svc:%d", cluster.Name, cluster.Namespace, etcdClientPort),
	}
	return &etcdclient.Config{
		Endpoints: []string{serviceURL.String()},
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
			Storage:     cluster.Spec.Storage.DeepCopy(),
		},
	}
}

// configurePeerBootstrap is used during *cluster* bootstrap to configure the peers to be able to see each other.
func configurePeerBootstrap(peer *etcdv1alpha1.EtcdPeer, cluster *etcdv1alpha1.EtcdCluster) {
	peer.Spec.Bootstrap = &etcdv1alpha1.Bootstrap{
		Static: &etcdv1alpha1.StaticBootstrap{
			InitialCluster: initialClusterMembers(cluster),
		},
		InitialClusterState: etcdv1alpha1.InitialClusterStateNew,
	}
}

func configureJoinExistingCluster(peer *etcdv1alpha1.EtcdPeer, cluster *etcdv1alpha1.EtcdCluster, members []etcdclient.Member) {
	peer.Spec.Bootstrap = &etcdv1alpha1.Bootstrap{
		Static: &etcdv1alpha1.StaticBootstrap{
			InitialCluster: initialClusterMembersFromMemberList(cluster, members),
		},
		InitialClusterState: etcdv1alpha1.InitialClusterStateExisting,
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

func initialClusterMembersFromMemberList(cluster *etcdv1alpha1.EtcdCluster, members []etcdclient.Member) []etcdv1alpha1.InitialClusterMember {
	initialMembers := make([]etcdv1alpha1.InitialClusterMember, len(members))
	for i, member := range members {
		name, err := peerNameForMember(member)
		if err != nil {
			// We were not able to figure out what this peer's name should be. There is precious little we can do here
			// so skip over this member.
			continue
		}
		initialMembers[i] = etcdv1alpha1.InitialClusterMember{
			Name: name,
			// The member actually will always have at least one peer URL set (in the case where we're inspecting a
			// member in the membership list that has never joined the cluster that is the only thing we'll see. But
			// using that is actually painful as it's often strangely escaped. We bypass that issue by regenerating our
			// expected url from the name.
			Host: expectedURLForPeer(cluster, name),
		}
	}
	return initialMembers
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
