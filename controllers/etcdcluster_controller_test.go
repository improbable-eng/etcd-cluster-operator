package controllers

import (
	"fmt"
	"github.com/improbable-eng/etcd-cluster-operator/internal/util/ptr"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

func (s *controllerSuite) testClusterController(t *testing.T) {
	t.Run("OnCreation", func(t *testing.T) {
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
				Replicas: ptr.Int32(3),
			},
		}

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		t.Run("CreatesService", func(t *testing.T) {
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

			// Check the service's labels
			require.Equal(t, "etcd", service.Labels["app.kubernetes.io/name"], "Service did not have app name label")
			require.Equal(t, etcdCluster.Name, service.Labels["etcd.improbable.io/cluster-name"], "Service did not have etcd cluster name label")

			// Assume single owner reference
			assertOwnedByCluster(t, etcdCluster, service)

			// Selector
			selector := service.Spec.Selector
			require.Equal(t, "etcd", selector["app.kubernetes.io/name"], "Selector did not select for 'etcd' app name label")
			require.Equal(t, etcdCluster.Name, selector["etcd.improbable.io/cluster-name"], "Selector did not select for etcd cluster name label")

			// Ports
			ports := service.Spec.Ports
			require.Contains(t, ports, v1.ServicePort{
				Name:       "etcd-client",
				Protocol:   "TCP",
				Port:       2379,
				TargetPort: intstr.FromInt(2379),
			}, "Service did not declare client port")
			require.Contains(t, ports, v1.ServicePort{
				Name:       "etcd-peer",
				Protocol:   "TCP",
				Port:       2380,
				TargetPort: intstr.FromInt(2380),
			}, "Service did not declare peer port")
		})

		t.Run("CreatesPeers", func(t *testing.T) {
			// Assert on peers
			peers := &etcdv1alpha1.EtcdPeerList{}
			err = try.Eventually(func() error {
				return s.k8sClient.List(s.ctx, peers, &client.ListOptions{
					Namespace: namespace,
				})
			}, time.Second*5, time.Millisecond*500)
			require.NoError(t, err)
			require.Lenf(t, peers.Items, 3, "wrong number of peers: %#v", peers)

			expectedInitialCluster := make([]etcdv1alpha1.InitialClusterMember, len(peers.Items))
			for i, peer := range peers.Items {
				expectedInitialCluster[i] = etcdv1alpha1.InitialClusterMember{
					Name: peer.Name,
					Host: fmt.Sprintf("%s.%s.%s.svc",
						peer.Name,
						etcdCluster.Name,
						namespace),
				}
			}

			for _, peer := range peers.Items {
				assertPeer(t, etcdCluster, &peer)

				assert.Equal(t,
					expectedInitialCluster,
					peer.Spec.Bootstrap.Static.InitialCluster,
					"Peer did not have expected static bootstrap instructions")
			}
		})
	})
}

func assertOwnedByCluster(t *testing.T, etcdCluster *etcdv1alpha1.EtcdCluster, obj metav1.Object) {
	require.Len(t, obj.GetOwnerReferences(), 1, "Incorrect number of owners")
	ownerRef := obj.GetOwnerReferences()[0]
	require.True(t, *ownerRef.Controller, "Did not have a controller owner reference")
	require.Equal(t, "etcd.improbable.io/v1alpha1", ownerRef.APIVersion)
	require.Equal(t, "EtcdCluster", ownerRef.Kind)
	require.Equal(t, etcdCluster.Name, ownerRef.Name)
}

func assertPeer(t *testing.T, cluster *etcdv1alpha1.EtcdCluster, peer *etcdv1alpha1.EtcdPeer) {
	require.Equal(t, cluster.Namespace, peer.Namespace, "Peer is not in same namespace as cluster")
	require.Contains(t, peer.Name, cluster.Name, "Peer name did not contain cluster's name")
	require.Equal(t, cluster.Name, peer.Spec.ClusterName, "Cluster name not set on peer")

	require.Equal(t, appName, peer.Labels[appLabel])
	require.Equal(t, cluster.Name, peer.Labels[clusterLabel])

	assertOwnedByCluster(t, cluster, peer)
}
