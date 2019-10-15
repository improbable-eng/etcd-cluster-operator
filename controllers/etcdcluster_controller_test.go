package controllers

import (
	v1 "k8s.io/api/core/v1"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

func (s *controllerSuite) testClusterController(t *testing.T) {
	t.Run("TestClusterController_OnCreation_CreatesReplicaSet", func(t *testing.T) {
		teardownFunc, namespace := s.setupTest(t)
		defer teardownFunc()

		etcdCluster := &etcdv1alpha1.EtcdCluster{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
				Name:        "bees",
				Namespace:   namespace,
			},
			Spec: etcdv1alpha1.EtcdClusterSpec{
				Replicas: 3,
			},
		}

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		// Assert on headless service
		service := &v1.Service{}
		err = try.Eventually(func() error {
			return s.k8sClient.Get(s.ctx, client.ObjectKey{
				Namespace: namespace,
				// Service will have the same name as the etcd cluster we asked to be created
				Name: etcdCluster.Name,
			},
				service)
		}, time.Second*5, time.Millisecond*500)
		require.NoError(t, err, "failed to find service for EtcdCluster")

		require.Equal(t, v1.ServiceTypeClusterIP, service.Spec.Type, "service was not a ClusterIP service")
		require.Equal(t, v1.ClusterIPNone, service.Spec.ClusterIP, "service was not a headless service")
		require.True(t, service.Spec.PublishNotReadyAddresses, "service did not publish not-ready addresses")

		// Assume single owner reference
		require.Len(t, service.OwnerReferences, 1, "Incorrect number of owners")
		ownerRef := service.OwnerReferences[0]
		require.True(t, *ownerRef.Controller, "Service did not have a controller owner reference")
		require.Equal(t, "etcd.improbable.io/v1alpha1", ownerRef.APIVersion)
		require.Equal(t, "EtcdCluster", ownerRef.Kind)
		require.Equal(t, etcdCluster.Name, ownerRef.Name)

		// Selector
		selector := service.Spec.Selector
		require.Equal(t, "etcd", selector["app.kubernetes.io/name"])
		require.Equal(t, etcdCluster.Name, selector["etcd.improbable.io/cluster-name"])

		// Ports
		ports := service.Spec.Ports
		require.Contains(t, ports, v1.ServicePort{
			Name:     "etcd-client",
			Protocol: "TCP",
			Port:     2379,
		})
		require.Contains(t, ports, v1.ServicePort{
			Name:     "etcd-peer",
			Protocol: "TCP",
			Port:     2380,
		})

		// Assert on peers
		//peers := &etcdv1alpha1.EtcdPeerList{}
		//
		//err = try.Eventually(func() error {
		//	return s.k8sClient.List(s.ctx, peers, &client.ListOptions{
		//		Namespace: namespace,
		//	})
		//}, time.Second*5, time.Millisecond*500)
		//require.NoError(t, err)
		//require.Lenf(t, peers.Items, 3, "wrong number of peers: %#v", peers)
	})
}