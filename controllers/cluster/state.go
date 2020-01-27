package cluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	etcdclient "go.etcd.io/etcd/client"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
)

type State struct {
	Cluster *etcdv1alpha1.EtcdCluster
	Members []etcdclient.Member
	Peers   *etcdv1alpha1.EtcdPeerList
}

func GetState(log logr.Logger, c client.Client, etcdapi etcd.EtcdAPI, ctx context.Context, req ctrl.Request) (*State, error) {
	state := &State{}

	var cluster etcdv1alpha1.EtcdCluster
	err := c.Get(ctx, req.NamespacedName, &cluster)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}
	if err == nil {
		state.Cluster = &cluster
	}

	if state.Cluster != nil {
		state.Cluster.Default()
	}

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

	// List peers
	var peers etcdv1alpha1.EtcdPeerList
	err = c.List(ctx, &peers, client.InNamespace(cluster.Namespace), client.MatchingFields{"spec.clusterName": cluster.Name})
	if err != nil {
		return nil, fmt.Errorf("unable to list peers: %s", err)
	}
	if err == nil {
		state.Peers = &peers
	}

	return state, nil
}
