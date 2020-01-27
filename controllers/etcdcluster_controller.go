package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	etcdclient "go.etcd.io/etcd/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	cl "github.com/improbable-eng/etcd-cluster-operator/controllers/cluster"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
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
	Etcd     etcd.EtcdAPI
}

func (r *EtcdClusterReconciler) createBootstrapPeer(ctx context.Context, cluster *etcdv1alpha1.EtcdCluster, peers *etcdv1alpha1.EtcdPeerList) (*reconcilerevent.PeerCreatedEvent, error) {
	peerName := nextAvailablePeerName(cluster, peers.Items)
	peer := peerForCluster(cluster, peerName)
	configurePeerBootstrap(peer, cluster)
	err := r.Create(ctx, peer)
	if err != nil {
		return nil, err
	}
	return &reconcilerevent.PeerCreatedEvent{Object: cluster, PeerName: peer.Name}, nil
}

func (r *EtcdClusterReconciler) hasPeerForMember(peers *etcdv1alpha1.EtcdPeerList, member etcdclient.Member) (bool, error) {
	expectedPeerName, err := peerNameForMember(member)
	if err != nil {
		return false, err
	}
	for _, peer := range peers.Items {
		if expectedPeerName == peer.Name {
			return true, nil
		}
	}
	return false, nil
}

func (r *EtcdClusterReconciler) createPeerForMember(ctx context.Context, cluster *etcdv1alpha1.EtcdCluster, members []etcdclient.Member, member etcdclient.Member) (*reconcilerevent.PeerCreatedEvent, error) {
	// Use this instead of member.Name. If we've just added the peer for this member then the member might not
	// have that field set, so instead figure it out from the peerURL.
	expectedPeerName, err := peerNameForMember(member)
	if err != nil {
		return nil, err
	}
	peer := peerForCluster(cluster, expectedPeerName)
	configureJoinExistingCluster(peer, cluster, members)

	err = r.Create(ctx, peer)
	if err != nil {
		return nil, err
	}
	return &reconcilerevent.PeerCreatedEvent{Object: cluster, PeerName: peer.Name}, nil
}

func (r *EtcdClusterReconciler) addNewMember(ctx context.Context, cluster *etcdv1alpha1.EtcdCluster, peers *etcdv1alpha1.EtcdPeerList) (*reconcilerevent.MemberAddedEvent, error) {
	peerName := nextAvailablePeerName(cluster, peers.Items)
	peerURL := &url.URL{
		Scheme: etcdv1alpha1.EtcdScheme,
		Host:   fmt.Sprintf("%s:%d", expectedURLForPeer(cluster, peerName), etcdv1alpha1.EtcdPeerPort),
	}
	member, err := r.addEtcdMember(ctx, cluster, peerURL.String())
	if err != nil {
		return nil, err
	}
	return &reconcilerevent.MemberAddedEvent{Object: cluster, Member: member, Name: peerName}, nil
}

// MembersByName provides a sort.Sort interface for etcdClient.Member.Name
type MembersByName []etcdclient.Member

func (a MembersByName) Len() int           { return len(a) }
func (a MembersByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a MembersByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

// removeMember selects the Etcd node which has a name with the largest ordinal,
// then performs a runtime reconfiguration to remove that member from the Etcd
// cluster, and generates an Event to record that action.
// TODO(wallrj) Consider removing a non-leader member to avoid disruptive
// leader elections while scaling down.
// See https://github.com/improbable-eng/etcd-cluster-operator/issues/97
func (r *EtcdClusterReconciler) removeMember(ctx context.Context, cluster *etcdv1alpha1.EtcdCluster, members []etcdclient.Member) (*reconcilerevent.MemberRemovedEvent, error) {
	sortedMembers := append(members[:0:0], members...)
	sort.Sort(sort.Reverse(MembersByName(sortedMembers)))
	member := sortedMembers[0]
	err := r.removeEtcdMember(ctx, cluster, member.ID)
	if err != nil {
		return nil, err
	}
	return &reconcilerevent.MemberRemovedEvent{Object: cluster, Member: &member, Name: member.Name}, nil
}

// updateStatus updates the EtcdCluster resource's status to be the current value of the cluster.
func (r *EtcdClusterReconciler) updateStatus(ctx context.Context,
	cluster *etcdv1alpha1.EtcdCluster,
	members []etcdclient.Member,
	peers *etcdv1alpha1.EtcdPeerList,
	reconcilerEvent reconcilerevent.ReconcilerEvent) error {

	updated := cluster.DeepCopy()

	if members != nil {
		updated.Status.Members = make([]etcdv1alpha1.EtcdMember, len(members))
		for i, member := range members {
			updated.Status.Members[i] = etcdv1alpha1.EtcdMember{
				Name: member.Name,
				ID:   member.ID,
			}
		}
	} else {
		if updated.Status.Members == nil {
			updated.Status.Members = make([]etcdv1alpha1.EtcdMember, 0)
		} else {
			// In this case we don't have up-to date members for whatever reason, which could be a temporary disruption
			// such as a network issue. But we also know the status on the resource already has a member list. So just
			// leave that (stale) data where it is and don't change it.
		}
	}
	updated.Status.Replicas = int32(len(peers.Items))

	if err := r.Client.Status().Patch(ctx, updated, client.MergeFrom(cluster)); err != nil {
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
	members []etcdclient.Member,
	peers *etcdv1alpha1.EtcdPeerList,
	cluster *etcdv1alpha1.EtcdCluster,
) (
	ctrl.Result,
	reconcilerevent.ReconcilerEvent,
	error,
) {
	// Use a logger enriched with information about the cluster we'er reconciling. Note we generally do not log on
	// errors as we pass the error up to the manager anyway.
	log := r.Log.WithValues("cluster", types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	})

	// Always requeue after ten seconds, as we don't watch on the membership list. So we don't auto-detect changes made
	// to the etcd membership API.
	// TODO(#76) Implement custom watch on etcd membership API, and remove this `requeueAfter`
	result := ctrl.Result{RequeueAfter: time.Second * 10}

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
		if hasTooFewPeers(cluster, peers) {
			// We have fewer peers than our replicas, so create a new peer. We do this one at a time, so only make
			// *one* peer.
			peerCreatedEvent, err := r.createBootstrapPeer(ctx, cluster, peers)
			if err != nil {
				return result, nil, fmt.Errorf("failed to create EtcdPeer %w", err)
			}
			log.V(1).Info(
				"Insufficient peers for replicas and unable to contact etcd. Assumed we're bootstrapping created new peer. (If we're not we'll heal this later.)",
				"current-peers", len(peers.Items),
				"desired-peers", cluster.Spec.Replicas,
				"peer", peerCreatedEvent.PeerName)
			return result, peerCreatedEvent, nil
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
		for _, member := range members {
			hasPeer, err := r.hasPeerForMember(peers, member)
			if err != nil {
				return result, nil, err
			}
			if !hasPeer {
				// We have found a member without a peer. We should add an EtcdPeer resource for this member
				peerCreatedEvent, err := r.createPeerForMember(ctx, cluster, members, member)
				if err != nil {
					return result, nil, fmt.Errorf("failed to create EtcdPeer %w", err)
				}
				log.V(1).Info(
					"Found member in etcd's API that has no EtcdPeer resource representation, added one.",
					"member-name", peerCreatedEvent.PeerName)
				return result, peerCreatedEvent, nil
			}
		}

		// If we've reached this point we're sure that the EtcdPeer resources in the cluster match the contents of the
		// membership API. The next step is checking to see if we want to mutate the membership API of etcd to scale up
		// or down. However we don't want to do this unless the cluster has stabilised from a previous member addition.
		if isAllMembersStable(members) {
			// It's finally time to see if we need to mutate the membership API to bring it in-line with our expected
			// replicas. There are three cases, we have enough members, too few, or too many. The order we check these
			// cases in is irrelevant as only one can possibly be true.
			if hasTooFewMembers(cluster, members) {
				// There are too few members for the expected number of replicas. Add a new member.
				memberAddedEvent, err := r.addNewMember(ctx, cluster, peers)
				if err != nil {
					return result, nil, fmt.Errorf("failed to add new member: %w", err)
				}

				log.V(1).Info("Too few members for expected replica count, adding new member.",
					"expected-replicas", *cluster.Spec.Replicas,
					"actual-members", len(members),
					"peer-name", memberAddedEvent.Name,
					"peer-url", memberAddedEvent.Member.PeerURLs[0])
				return result, memberAddedEvent, nil
			} else if hasTooManyMembers(cluster, members) {
				// There are too many members for the expected number of replicas.
				// Remove the member with the highest ordinal.
				memberRemovedEvent, err := r.removeMember(ctx, cluster, members)
				if err != nil {
					return result, nil, fmt.Errorf("failed to remove member: %w", err)
				}

				log.V(1).Info("Too many members for expected replica count, removing member.",
					"expected-replicas", *cluster.Spec.Replicas,
					"actual-members", len(members),
					"peer-name", memberRemovedEvent.Name,
					"peer-url", memberRemovedEvent.Member.PeerURLs[0])

				return result, memberRemovedEvent, nil
			} else {
				// Exactly the correct number of members. Make no change, all is well with the world.
				log.V(2).Info("Expected number of replicas aligned with actual number.",
					"expected-replicas", *cluster.Spec.Replicas,
					"actual-members", len(members))
			}

			// Remove EtcdPeer resources which do not have members.
			memberNames := sets.NewString()
			for _, member := range members {
				memberNames.Insert(member.Name)
			}

			for _, peer := range peers.Items {
				if !memberNames.Has(peer.Name) {
					if !peer.DeletionTimestamp.IsZero() {
						log.V(2).Info("Peer is already marked for deletion", "peer", peer.Name)
						continue
					}
					if !hasPvcDeletionFinalizer(&peer) {
						updated := peer.DeepCopy()
						controllerutil.AddFinalizer(updated, etcdv1alpha1.PVCCleanupFinalizer)
						err := r.Patch(ctx, updated, client.MergeFrom(&peer))
						if err != nil {
							return result, nil, fmt.Errorf("failed to add PVC cleanup finalizer: %w", err)
						}
						log.V(2).Info("Added PVC cleanup finalizer", "peer", peer.Name)
						return result, nil, nil
					}
					err := r.Delete(ctx, &peer)
					if err != nil {
						return result, nil, fmt.Errorf("failed to remove peer: %w", err)
					}
					peerRemovedEvent := &reconcilerevent.PeerRemovedEvent{
						Object:   &peer,
						PeerName: peer.Name,
					}
					return result, peerRemovedEvent, nil
				}
			}
		}
	}
	// This is the fall-through 'all is right with the world' case.
	return result, nil, nil
}

func hasTooFewPeers(cluster *etcdv1alpha1.EtcdCluster, peers *etcdv1alpha1.EtcdPeerList) bool {
	return cluster.Spec.Replicas != nil && int32(len(peers.Items)) < *cluster.Spec.Replicas
}

func hasTooFewMembers(cluster *etcdv1alpha1.EtcdCluster, members []etcdclient.Member) bool {
	return *cluster.Spec.Replicas > int32(len(members))
}

func hasTooManyMembers(cluster *etcdv1alpha1.EtcdCluster, members []etcdclient.Member) bool {
	return *cluster.Spec.Replicas < int32(len(members))
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

func (r *EtcdClusterReconciler) nextAction(log logr.Logger, state *cl.State) Action {
	if state.Cluster == nil {
		log.Info("EtcdCluster not found")
		return nil
	}

	// Validate in case a validating webhook has not been deployed
	err := state.Cluster.ValidateCreate()
	if err != nil {
		log.Error(err, "Invalid EtcdCluster")
		return nil
	}

	var (
		action Action
		event  reconcilerevent.ReconcilerEvent
	)
	switch {
	case state.Service == nil:
		// The first port of call is to verify that the service exists and create it if it does not.
		//
		// This service is headless, and is designed to provide the underlying etcd pods with a consistent network identity
		// such that they can dial each other. It *can* be used for client access to etcd if a user desires, but users are
		// also encouraged to create their own services for their client access if that is more convenient.
		action = &CreateRuntimeObject{log: log, client: r.Client, obj: state.DesiredService}
		event = &reconcilerevent.ServiceCreatedEvent{Object: state.Cluster, ServiceName: state.DesiredService.Name}
	}

	if event != nil {
		action = &EventWrapper{
			recorder: r.Recorder,
			event:    event,
			wrapped:  action,
		}
	}

	return action
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;create;delete;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *EtcdClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("cluster", req.NamespacedName)

	state, err := cl.GetState(log, r.Client, r.Etcd, ctx, req)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error while getting current state: %s", err)
	}

	// Always requeue after ten seconds, as we don't watch on the membership list. So we don't auto-detect changes made
	// to the etcd membership API.
	// TODO(#76) Implement custom watch on etcd membership API, and remove this `requeueAfter`
	action := r.nextAction(log, state)
	if action != nil {
		return ctrl.Result{RequeueAfter: time.Second * 10}, action.Execute(ctx)
	}

	// Perform a reconcile, getting back the desired result, any utilerrors, and a clusterEvent. This is an internal concept
	// and is not the same as the Kubernetes event, although it is used to make one later.
	result, clusterEvent, reconcileErr := r.reconcile(ctx, state.Members, state.Peers, state.Cluster)
	if reconcileErr != nil {
		log.Error(reconcileErr, "Failed to reconcile")
	}

	// The update status takes in the cluster definition, and the member list from etcd as of *before we ran reconcile*.
	// We also get the event, which may contain rich information about what we did (such as the new member name on a
	// MemberAdded event).
	updateStatusErr := r.updateStatus(ctx, state.Cluster, state.Members, state.Peers, clusterEvent)
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
	c, err := r.Etcd.MembershipAPI(cl.EtcdClientConfig(cluster))
	if err != nil {
		return nil, fmt.Errorf("unable to connect to etcd: %w", err)
	}

	members, err := c.Add(ctx, peerURL)
	if err != nil {
		return nil, fmt.Errorf("unable to add member to etcd cluster: %w", err)
	}

	return members, nil
}

// removeEtcdMember performs a runtime reconfiguration of the Etcd cluster to
// remove a member from the cluster.
func (r *EtcdClusterReconciler) removeEtcdMember(ctx context.Context, cluster *etcdv1alpha1.EtcdCluster, memberID string) error {
	c, err := r.Etcd.MembershipAPI(cl.EtcdClientConfig(cluster))
	if err != nil {
		return fmt.Errorf("unable to connect to etcd: %w", err)
	}
	err = c.Remove(ctx, memberID)
	if err != nil {
		return fmt.Errorf("unable to remove member from etcd cluster: %w", err)
	}

	return nil
}

func peerForCluster(cluster *etcdv1alpha1.EtcdCluster, peerName string) *etcdv1alpha1.EtcdPeer {
	peer := &etcdv1alpha1.EtcdPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peerName,
			Namespace: cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cluster, etcdv1alpha1.GroupVersion.WithKind("EtcdCluster")),
			},
			Labels: map[string]string{
				etcdv1alpha1.AppLabel:     etcdv1alpha1.AppName,
				etcdv1alpha1.ClusterLabel: cluster.Name,
			},
		},
		Spec: etcdv1alpha1.EtcdPeerSpec{
			ClusterName: cluster.Name,
			Storage:     cluster.Spec.Storage.DeepCopy(),
		},
	}

	if cluster.Spec.PodTemplate != nil {
		peer.Spec.PodTemplate = cluster.Spec.PodTemplate
	}

	return peer
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
