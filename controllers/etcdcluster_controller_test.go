package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	etcdclient "go.etcd.io/etcd/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

type AlwaysFailEtcdAPI struct{}

func (_ *AlwaysFailEtcdAPI) MembershipAPI(_ etcdclient.Config) (etcdclient.MembersAPI, error) {
	return nil, errors.New("fake etcd, nothing is here")
}

type StaticResponseMembersAPI struct {
	Members []etcdclient.Member
}

func (s StaticResponseMembersAPI) List(ctx context.Context) ([]etcdclient.Member, error) {
	return s.Members, nil
}

func (s StaticResponseMembersAPI) Add(ctx context.Context, peerURL string) (*etcdclient.Member, error) {
	panic("implement me")
}

func (s StaticResponseMembersAPI) Remove(ctx context.Context, mID string) error {
	panic("implement me")
}

func (s StaticResponseMembersAPI) Update(ctx context.Context, mID string, peerURLs []string) error {
	panic("implement me")
}

func (s StaticResponseMembersAPI) Leader(ctx context.Context) (*etcdclient.Member, error) {
	panic("implement me")
}

type StaticResponseEtcdAPI struct {
	Members []etcdclient.Member
}

func (sr *StaticResponseEtcdAPI) MembershipAPI(_ etcdclient.Config) (etcdclient.MembersAPI, error) {
	return &StaticResponseMembersAPI{Members: sr.Members}, nil
}

func (s *controllerSuite) testClusterController(t *testing.T) {
	t.Run("OnCreation", func(t *testing.T) {
		teardownFunc, namespace := s.setupTest(t)
		defer teardownFunc()

		etcdCluster := test.ExampleEtcdCluster(namespace)

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		// Apply defaults here so that our expected object has all the same
		// defaults as those used in the Reconcile function
		etcdCluster.Default()

		// Mock out the etcd API with one that always fails - i.e., we're always in 'bootstrap' mode
		s.etcd = &AlwaysFailEtcdAPI{}

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

				assert.ElementsMatch(t,
					expectedInitialCluster,
					peer.Spec.Bootstrap.Static.InitialCluster,
					"Peer did not have expected static bootstrap instructions")
			}
		})

		t.Run("UpdatesStatus", func(t *testing.T) {
			// Make our fake etcd respond
			members := make([]etcdclient.Member, *etcdCluster.Spec.Replicas)
			for i := range members {
				name := fmt.Sprintf("%s-%d", etcdCluster.Name, i)
				peerURL := &url.URL{
					Scheme: etcdScheme,
					Host: fmt.Sprintf("%s.%s.%s.svc:%d",
						name,
						etcdCluster.Name,
						etcdCluster.Namespace,
						etcdPeerPort,
					),
				}
				clientURL := &url.URL{
					Scheme: etcdScheme,
					Host: fmt.Sprintf("%s.%s.%s.svc:%d",
						name,
						etcdCluster.Name,
						etcdCluster.Namespace,
						etcdClientPort,
					),
				}
				members[i] = etcdclient.Member{
					ID:         fmt.Sprintf("SOMEID%d", i),
					Name:       name,
					PeerURLs:   []string{peerURL.String()},
					ClientURLs: []string{clientURL.String()},
				}
			}
			s.etcd = &StaticResponseEtcdAPI{Members: members}

			err = try.Eventually(func() error {
				fetchedCluster := &etcdv1alpha1.EtcdCluster{}
				err := s.k8sClient.Get(s.ctx,
					client.ObjectKey{
						Namespace: namespace,
						Name:      etcdCluster.Name,
					},
					fetchedCluster)
				if err != nil {
					return err
				}
				for _, expectedMember := range members {
					// Ensure our expected member is in this list
					foundMember := false
					for _, actualMember := range fetchedCluster.Status.Members {
						if actualMember.Name == expectedMember.Name {
							foundMember = true
							continue
						}
					}
					if !foundMember {
						return errors.New(fmt.Sprintf("failed to find member %s", expectedMember.Name))
					}
				}
				// All good
				return nil
			}, time.Second*30, time.Millisecond*500)
			require.NoError(t, err)
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

	assert.Equal(t, cluster.Spec.Storage, peer.Spec.Storage, "unexpected peer storage")
}
