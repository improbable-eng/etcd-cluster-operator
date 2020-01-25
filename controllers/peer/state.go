package peer

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

type State struct {
	Peer              *etcdv1alpha1.EtcdPeer
	PVC               *corev1.PersistentVolumeClaim
	DesiredPVC        *corev1.PersistentVolumeClaim
	ReplicaSet        *appsv1.ReplicaSet
	DesiredReplicaSet *appsv1.ReplicaSet
}

func GetState(log logr.Logger, c client.Client, ctx context.Context, req ctrl.Request) (*State, error) {
	state := &State{}

	var peer etcdv1alpha1.EtcdPeer
	err := c.Get(ctx, req.NamespacedName, &peer)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}
	if err == nil {
		state.Peer = &peer
	}

	var pvc corev1.PersistentVolumeClaim
	err = c.Get(ctx, req.NamespacedName, &pvc)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}
	if err == nil {
		state.PVC = &pvc
	}

	var replicaSet appsv1.ReplicaSet
	err = c.Get(ctx, req.NamespacedName, &replicaSet)
	if client.IgnoreNotFound(err) != nil {
		return nil, err
	}
	if err == nil {
		state.ReplicaSet = &replicaSet
	}

	if state.Peer != nil {
		state.Peer.Default()
		state.DesiredPVC = pvcForPeer(state.Peer)
		state.DesiredReplicaSet = defineReplicaSet(state.Peer, log)
	}

	return state, nil
}
