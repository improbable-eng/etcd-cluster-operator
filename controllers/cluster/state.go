package cluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	etcdclient "go.etcd.io/etcd/client"
	v1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
)

type State struct {
	Cluster        *etcdv1alpha1.EtcdCluster
	Members        []etcdclient.Member
	Peers          *etcdv1alpha1.EtcdPeerList
	DesiredService *v1.Service
	Service        *v1.Service
}

func GetState(log logr.Logger, c client.Client, etcdapi etcd.EtcdAPI, ctx context.Context, req ctrl.Request) (*State, error) {
	state := &State{}

	var cluster etcdv1alpha1.EtcdCluster
	err := c.Get(ctx, req.NamespacedName, &cluster)
	if err != nil {
		return nil, err
	}

	state.Cluster = &cluster
	state.Cluster.Default()

	// List peers
	var peers etcdv1alpha1.EtcdPeerList
	err = c.List(ctx, &peers, client.InNamespace(cluster.Namespace), client.MatchingFields{"spec.clusterName": cluster.Name})
	if err != nil {
		return nil, fmt.Errorf("unable to list peers: %s", err)
	}
	if err == nil {
		state.Peers = &peers
	}

	var service v1.Service
	err = c.Get(ctx, serviceName(&cluster), &service)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}
	if err == nil {
		state.Service = &service
	}

	state.DesiredService = headlessServiceForCluster(&cluster)

	etcdclient, err := etcdapi.MembershipAPI(EtcdClientConfig(&cluster))
	if err == nil {
		members, err := etcdclient.List(ctx)
		if err == nil {
			state.Members = members
		} else {
			log.Error(err, "unable to list members of etcd cluster")
		}
	} else {
		log.Error(err, "unable to connect to etcd")
	}

	return state, nil
}

func ClusterWithUpdatedStatus(original *etcdv1alpha1.EtcdCluster, state *State) *etcdv1alpha1.EtcdCluster {
	updated := original.DeepCopy()
	updated.Status.Replicas = int32(len(state.Peers.Items))
	updated.Status.Members = make([]etcdv1alpha1.EtcdMember, len(state.Members))
	for i, member := range state.Members {
		updated.Status.Members[i] = etcdv1alpha1.EtcdMember{
			Name: member.Name,
			ID:   member.ID,
		}
	}
	return updated
}
