package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	etcdclient "go.etcd.io/etcd/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// MembersByName provides a sort.Sort interface for etcdClient.Member.Name
type MembersByName []etcdclient.Member

func (a MembersByName) Len() int           { return len(a) }
func (a MembersByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a MembersByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

// memberToRemove selects the Etcd node which has a name with the largest ordinal,
// TODO(wallrj) Consider removing a non-leader member to avoid disruptive
// leader elections while scaling down.
// See https://github.com/improbable-eng/etcd-cluster-operator/issues/97
func memberToRemove(members []etcdclient.Member) etcdclient.Member {
	sortedMembers := append(members[:0:0], members...)
	sort.Sort(sort.Reverse(MembersByName(sortedMembers)))
	member := sortedMembers[0]
	return member
}

func peerToRemove(peers *etcdv1alpha1.EtcdPeerList, members []etcdclient.Member) *etcdv1alpha1.EtcdPeer {
	// Remove EtcdPeer resources which do not have members.
	memberNames := sets.NewString()
	for _, member := range members {
		memberNames.Insert(member.Name)
	}

	for _, peer := range peers.Items {
		if !memberNames.Has(peer.Name) {
			return &peer
		}
	}
	return nil
}

func hasTooFewPeers(cluster *etcdv1alpha1.EtcdCluster, peers *etcdv1alpha1.EtcdPeerList) bool {
	return cluster.Spec.Replicas != nil && int32(len(peers.Items)) < *cluster.Spec.Replicas
}

func hasTooManyPeers(cluster *etcdv1alpha1.EtcdCluster, peers *etcdv1alpha1.EtcdPeerList) bool {
	return cluster.Spec.Replicas != nil && int32(len(peers.Items)) > *cluster.Spec.Replicas
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
	case !state.Cluster.DeletionTimestamp.IsZero():
		// Cluster is being deleted. Do nothing.

	case state.Service == nil:
		// The first port of call is to verify that the service exists and create it if it does not.
		//
		// This service is headless, and is designed to provide the underlying etcd pods with a consistent network identity
		// such that they can dial each other. It *can* be used for client access to etcd if a user desires, but users are
		// also encouraged to create their own services for their client access if that is more convenient.
		action = &CreateRuntimeObject{log: log, client: r.Client, obj: state.DesiredService}
		event = &reconcilerevent.ServiceCreatedEvent{Object: state.Cluster, ServiceName: state.DesiredService.Name}

	case state.Members == nil && hasTooFewPeers(state.Cluster, state.Peers):
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
		peerName := nextAvailablePeerName(state.Cluster, state.Peers.Items)
		peer := peerForCluster(state.Cluster, peerName)
		configurePeerBootstrap(peer, state.Cluster)
		action = &CreateRuntimeObject{log: log, client: r.Client, obj: peer}
		event = &reconcilerevent.PeerCreatedEvent{Object: state.Cluster, PeerName: peer.Name}
		log.V(1).Info(
			"Insufficient peers for replicas and unable to contact etcd. Assumed we're bootstrapping created new peer. (If we're not we'll heal this later.)",
			"current-peers", len(state.Peers.Items),
			"desired-peers", state.Cluster.Spec.Replicas,
			"peer", peer.Name,
		)

	case state.Members == nil:
		// We can't contact the cluster, and we don't *think* we need more peers. So it's unsafe to take any
		// action.
		// In all the following cases, we *do* have communication with the cluster, and have retrieved a list of members. We treat
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

	case !isAllMembersStable(state.Members) && hasTooFewPeers(state.Cluster, state.Peers):
		peer, err := firstMissingMemberPeer(state)
		if err != nil {
			log.Error(err, "Error while computing missing member peer")
			break
		}
		action = &CreateRuntimeObject{log: log, client: r.Client, obj: peer}
		event = &reconcilerevent.PeerCreatedEvent{Object: state.Cluster, PeerName: peer.Name}
		log.V(1).Info(
			"Found member in etcd's API that has no EtcdPeer resource representation, added one.",
			"peer-name", peer.Name)

	case !isAllMembersStable(state.Members):
		// The cluster is not stable. Do not perform any more actions.

		// It's finally time to see if we need to mutate the membership API to bring it in-line with our expected
		// replicas. There are three cases, we have enough members, too few, or too many. The order we check these
		// cases in is irrelevant as only one can possibly be true.

	case hasTooFewMembers(state.Cluster, state.Members):
		// There are too few members for the expected number of replicas. Add a new member.
		peerName := nextAvailablePeerName(state.Cluster, state.Peers.Items)
		peerURL := expectedURLForPeer(state.Cluster, peerName)
		action = &AddEtcdMember{log: log, config: cl.EtcdClientConfig(state.Cluster), etcd: r.Etcd, url: peerURL}
		event = &reconcilerevent.MemberAddedEvent{Object: state.Cluster, Name: peerName}
		log.V(1).Info("Too few members for expected replica count, adding new member.",
			"expected-replicas", *state.Cluster.Spec.Replicas,
			"actual-members", len(state.Members),
			"peer-name", peerName,
			"peer-url", peerURL)

	case hasTooManyMembers(state.Cluster, state.Members):
		member := memberToRemove(state.Members)
		action = &RemoveEtcdMember{log: log, config: cl.EtcdClientConfig(state.Cluster), etcd: r.Etcd, mID: member.ID}
		event = &reconcilerevent.MemberRemovedEvent{Object: state.Cluster, Member: &member, Name: member.Name}
		log.V(1).Info("Too many members for expected replica count, removing member.",
			"expected-replicas", *state.Cluster.Spec.Replicas,
			"actual-members", len(state.Members),
			"member-name", member.Name,
			"member-id", member.ID)

	case hasTooManyPeers(state.Cluster, state.Peers):
		peer := peerToRemove(state.Peers, state.Members)
		if peer == nil {
			break
		}
		action = &RemoveEtcdPeer{log: log, client: r.Client, peer: peer}
		event = &reconcilerevent.PeerRemovedEvent{Object: state.Cluster, PeerName: peer.Name}
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

func (r *EtcdClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("cluster", req.NamespacedName)

	state, err := cl.GetState(log, r.Client, r.Etcd, ctx, req)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error while getting current state: %s", err)
	}
	if state == nil {
		return ctrl.Result{}, nil
	}
	defer func() {
		// Always attempt to patch the status after each reconciliation.
		log.Info("Patching status")
		original := state.Cluster
		updated := cl.ClusterWithUpdatedStatus(original, state)
		if reflect.DeepEqual(original.Status, updated.Status) {
			return
		}
		patch := client.MergeFrom(original)
		if err := r.Client.Status().Patch(ctx, updated, patch); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, fmt.Errorf("error while patching EtcdCluster.Status: %s ", err)})
		}
	}()

	// Always requeue after ten seconds, as we don't watch on the membership list. So we don't auto-detect changes made
	// to the etcd membership API.
	// TODO(#76) Implement custom watch on etcd membership API, and remove this `requeueAfter`
	result := ctrl.Result{RequeueAfter: time.Second * 10}
	action := r.nextAction(log, state)
	if action != nil {
		return result, action.Execute(ctx)
	}
	return result, nil
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
			Host: expectedURLForPeer(cluster, names[i]).Hostname(),
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
			Host: expectedURLForPeer(cluster, name).Hostname(),
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
func expectedURLForPeer(cluster *etcdv1alpha1.EtcdCluster, peerName string) *url.URL {
	return &url.URL{
		Scheme: etcdv1alpha1.EtcdScheme,
		Host:   fmt.Sprintf("%s.%s.%s.svc:%d", peerName, cluster.Name, cluster.Namespace, etcdv1alpha1.EtcdPeerPort),
	}
}

func firstMissingMemberPeer(state *cl.State) (*etcdv1alpha1.EtcdPeer, error) {
	peerUrls := make([]string, len(state.Peers.Items))
	for i, peer := range state.Peers.Items {
		peerUrls[i] = expectedURLForPeer(state.Cluster, peer.Name).String()
	}
	peerUrlsSet := sets.NewString(peerUrls...)

	memberUrls := make([]string, len(state.Members))
	for i, member := range state.Members {
		memberURL, err := url.Parse(member.PeerURLs[0])
		if err != nil {
			return nil, err
		}
		memberUrls[i] = memberURL.String()

		if peerUrlsSet.Has(memberURL.String()) {
			continue
		}
		var (
			peerName         string
			clusterName      string
			clusterNamespace string
		)
		hostname := memberURL.Hostname()
		labels := strings.SplitN(hostname, ".", 4)
		if len(labels) != 4 {
			return nil, fmt.Errorf("expected 4 labels in the member hostname: %s", hostname)
		}
		if labels[3] != "svc" {
			return nil, fmt.Errorf("expected member hostname to end with .svc: %s", hostname)
		}
		peerName, clusterName, clusterNamespace = labels[0], labels[1], labels[2]
		if clusterName != state.Cluster.Name {
			return nil, fmt.Errorf("expected second hostname label %s to be %s: %s", clusterName, state.Cluster.Name, hostname)
		}
		if clusterNamespace != state.Cluster.Namespace {
			return nil, fmt.Errorf("expected third hostname label %s to be %s: %s", clusterNamespace, state.Cluster.Namespace, hostname)
		}
		peer := peerForCluster(state.Cluster, peerName)
		configureJoinExistingCluster(peer, state.Cluster, state.Members)
		return peer, nil
	}
	return nil, fmt.Errorf("unable to find member without a peer. memberUrls: %v, peerUrls: %v", memberUrls, peerUrls)
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
