package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	etcdclient "go.etcd.io/etcd/client"
	"go.etcd.io/etcd/version"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

type AlwaysFailEtcdAPI struct{}

var _ etcd.APIBuilder = &AlwaysFailEtcdAPI{}

func (_ *AlwaysFailEtcdAPI) New(_ etcdclient.Config) (etcd.API, error) {
	return nil, errors.New("fake etcd, nothing is here")
}

// deepCopyEtcdClientMember makes a copy of the supplied Member
func deepCopyEtcdClientMember(in etcdclient.Member) (out etcdclient.Member, err error) {
	encoded, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(encoded, &out)
	return out, err
}

// StaticResponseEtcdAPI can be injected into the etcdcluster_controller for
// testing the interactions with the Etcd client API.
type StaticResponseEtcdAPI struct {
	sync.RWMutex
	Members        []etcdclient.Member
	ClusterVersion string
	ServerVersion  string
}

var _ etcd.APIBuilder = &StaticResponseEtcdAPI{}

func (s *StaticResponseEtcdAPI) New(config etcdclient.Config) (etcd.API, error) {
	return s, nil
}

var _ etcd.API = &StaticResponseEtcdAPI{}

func (s *StaticResponseEtcdAPI) List(ctx context.Context) ([]etcdclient.Member, error) {
	s.RLock()
	defer s.RUnlock()
	out := make([]etcdclient.Member, len(s.Members))
	for i := 0; i < len(out); i++ {
		new, err := deepCopyEtcdClientMember(s.Members[i])
		if err != nil {
			return nil, err
		}
		out[i] = new
	}
	return out, nil
}

func (s *StaticResponseEtcdAPI) Add(ctx context.Context, peerURL string) (*etcdclient.Member, error) {
	s.Lock()
	defer s.Unlock()
	panic("implement me")
}

func (s *StaticResponseEtcdAPI) Remove(ctx context.Context, mID string) error {
	s.Lock()
	defer s.Unlock()
	for i := range s.Members {
		if s.Members[i].ID == mID {
			s.Members = append(s.Members[:i], s.Members[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("unknown member: %s", mID)
}

func (s *StaticResponseEtcdAPI) GetVersion(ctx context.Context) (*version.Versions, error) {
	s.RLock()
	defer s.RUnlock()
	return &version.Versions{
		Cluster: s.ClusterVersion,
		Server:  s.ServerVersion,
	}, nil
}

type WrapperEtcdAPI struct {
	sync.RWMutex
	wrapped etcd.APIBuilder
}

var _ etcd.APIBuilder = &WrapperEtcdAPI{}

func (o *WrapperEtcdAPI) New(config etcdclient.Config) (etcd.API, error) {
	o.RLock()
	defer o.RUnlock()
	return o.wrapped.New(config)
}

func (o *WrapperEtcdAPI) Wrap(wrapped etcd.APIBuilder) {
	o.Lock()
	defer o.Unlock()
	o.wrapped = wrapped
}

// fakeEtcdForEtcdCluster returns a fake MembersAPI which simulates an Etcd API
// which lists all the members of the cluster.
// I.e. An established and healthy cluster rather than a cluster which is being bootstrapped.
func fakeEtcdForEtcdCluster(etcdCluster etcdv1alpha1.EtcdCluster) *StaticResponseEtcdAPI {
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
	return &StaticResponseEtcdAPI{
		Members:        members,
		ClusterVersion: etcdCluster.Spec.Version,
		ServerVersion:  etcdCluster.Spec.Version,
	}
}

func (s *controllerSuite) testClusterController(t *testing.T) {
	t.Run("ScaleDown", func(t *testing.T) {
		t.Log("Setup.")
		etcdAPI := &WrapperEtcdAPI{wrapped: &AlwaysFailEtcdAPI{}}

		teardownFunc, namespace := s.setupTest(t, etcdAPI)
		defer teardownFunc()

		etcdCluster := test.ExampleEtcdCluster(namespace)
		// Apply defaults here so that our expected object has all the same
		// defaults as those used in the Reconcile function
		etcdCluster.Default()

		const originalReplicas = 3

		*etcdCluster.Spec.Replicas = originalReplicas
		etcdAPIStatic := fakeEtcdForEtcdCluster(*etcdCluster)
		etcdAPI.Wrap(etcdAPIStatic)

		t.Log("Given an established 3-node cluster.")
		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		clusterObjectKey := client.ObjectKey{
			Namespace: namespace,
			Name:      etcdCluster.Name,
		}
		err = try.Eventually(func() error {
			var updatedCluster etcdv1alpha1.EtcdCluster
			err := s.k8sClient.Get(s.ctx, clusterObjectKey, &updatedCluster)
			require.NoError(t, err, "failed to find EtcdCluster")
			if updatedCluster.Status.Replicas != *etcdCluster.Spec.Replicas {
				return fmt.Errorf(
					"unexpected replicas count in cluster status. replicas: %d, status: %#v",
					updatedCluster.Status.Replicas, updatedCluster.Status,
				)
			}
			return nil
		}, time.Second*5, time.Millisecond*500)
		require.NoError(t, err)

		t.Log("When the cluster is scaled down to 1-node.")
		patch := client.MergeFrom(etcdCluster.DeepCopy())
		*etcdCluster.Spec.Replicas = 1
		err = s.k8sClient.Patch(s.ctx, etcdCluster, patch)
		require.NoError(t, err, "failed to patch EtcdCluster resource")

		t.Run("StatusUpdate", func(t *testing.T) {
			t.Log("The cluster status is updated when the cluster has been scaled down.")
			expectedStatus := expectedStatusForCluster(*etcdCluster)
			err = try.Eventually(func() error {
				var updatedCluster etcdv1alpha1.EtcdCluster
				err := s.k8sClient.Get(s.ctx, client.ObjectKey{
					Namespace: namespace,
					Name:      etcdCluster.Name,
				}, &updatedCluster)
				require.NoError(t, err, "failed to find EtcdCluster")

				if diff := cmp.Diff(expectedStatus, updatedCluster.Status); diff != "" {
					return fmt.Errorf("unexpected EtcdCluster.Status: diff --- expected, +++ actual\n%s", diff)
				}
				return nil
			}, time.Second*20, time.Millisecond*500)
			assert.NoError(t, err)
		})
		t.Run("EtcdMembersUpdate", func(t *testing.T) {
			t.Log("The etcd cluster API reports the expected number of nodes")
			membership, err := etcdAPI.New(etcdclient.Config{})
			require.NoError(t, err)
			expectedMembers := expectedEtcdMembersForCluster(*etcdCluster)
			err = try.Eventually(func() error {
				members, err := membership.List(s.ctx)
				require.NoError(t, err)
				if diff := cmp.Diff(expectedMembers, members); diff != "" {
					return fmt.Errorf("unexpected Etcd API members: diff --- expected, +++ actual\n%s", diff)
				}
				return nil
			}, time.Second*20, time.Millisecond*500)
			assert.NoError(t, err)
		})
		t.Run("EtcdPeersRemoved", func(t *testing.T) {
			t.Log("The EtcdPeers are removed")
			expected := expectedEtcdPeersForCluster(*etcdCluster)
			err = try.Eventually(func() error {
				var actual etcdv1alpha1.EtcdPeerList
				err := s.k8sClient.List(s.ctx, &actual, client.InNamespace(etcdCluster.Namespace))
				require.NoError(t, err)
				return hasIdenticalListItemNames(&expected, &actual)
			}, time.Second*20, time.Millisecond*500)
			assert.NoError(t, err)
		})
		t.Run("PersistentVolumeClaimsRemoved", func(t *testing.T) {
			t.Log("The PersistentVolumeClaims are removed")
			expected := expectedEtcdPeersForCluster(*etcdCluster)
			err = try.Eventually(func() error {
				var actual v1.PersistentVolumeClaimList
				err := s.k8sClient.List(s.ctx, &actual, client.InNamespace(etcdCluster.Namespace))
				require.NoError(t, err)
				return hasIdenticalListItemNames(&expected, &actual)
			}, time.Second*20, time.Millisecond*500)
			assert.NoError(t, err)
		})
	})

	t.Run("OnCreation", func(t *testing.T) {
		// Mock out the etcd API with one that always fails - i.e., we're always
		// in 'bootstrap' mode.
		// But use a wrapper so that the implementation can be switched later.
		etcdAPI := &WrapperEtcdAPI{wrapped: &AlwaysFailEtcdAPI{}}
		teardownFunc, namespace := s.setupTest(t, etcdAPI)
		defer teardownFunc()

		const expectedReplicas = 3

		etcdCluster := test.ExampleEtcdCluster(namespace)
		etcdCluster.Spec.Replicas = pointer.Int32Ptr(expectedReplicas)

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		// Apply defaults here so that our expected object has all the same
		// defaults as those used in the Reconcile function
		etcdCluster.Default()

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
				err := s.k8sClient.List(s.ctx, peers, &client.ListOptions{
					Namespace: namespace,
				})
				if err != nil {
					return err
				}
				if len(peers.Items) != expectedReplicas {
					return fmt.Errorf("wrong number of peers. expected: %d, actual: %d", expectedReplicas, len(peers.Items))
				}
				return nil
			}, time.Second*5, time.Millisecond*500)
			require.NoError(t, err)

			expectedInitialCluster := make([]etcdv1alpha1.InitialClusterMember, len(peers.Items))
			for i, peer := range peers.Items {
				expectedInitialCluster[i] = etcdv1alpha1.InitialClusterMember{
					Name: peer.Name,
					Host: fmt.Sprintf("%s.%s", peer.Name, etcdCluster.Name),
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

			// Swap the wrapped implementation, to one which responds with a
			// fixed list of members
			etcdAPI.Wrap(&StaticResponseEtcdAPI{Members: members})

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

	t.Run("PodAnnotations", func(t *testing.T) {
		// Mock out the etcd API with one that always fails - i.e., we're always in 'bootstrap' mode
		teardownFunc, namespace := s.setupTest(t, &AlwaysFailEtcdAPI{})
		defer teardownFunc()

		etcdCluster := test.ExampleEtcdCluster(namespace)

		expectedAnnotations := map[string]string{
			"foo":                "bar",
			"prometheus.io/path": "/_metrics",
		}

		etcdCluster.Spec.PodTemplate = &etcdv1alpha1.EtcdPodTemplateSpec{
			Metadata: &etcdv1alpha1.EtcdPodTemplateObjectMeta{
				Annotations: expectedAnnotations,
			},
		}

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		// Apply defaults here so that our expected object has all the same
		// defaults as those used in the Reconcile function
		etcdCluster.Default()

		t.Run("AppliesAnnotationsToPod", func(t *testing.T) {
			// Search for etcd pods using the clusterLabel
			replicaSetList := &appsv1.ReplicaSetList{}
			err = try.Eventually(func() error {
				err := s.k8sClient.List(s.ctx, replicaSetList,
					client.InNamespace(namespace),
				)
				t.Log(fmt.Sprintf("%v", replicaSetList))
				if len(replicaSetList.Items) != 3 {
					return errors.New(fmt.Sprintf("Wrong number of etcd Replica Sets. Had %d wanted %d", len(replicaSetList.Items), 3))
				}
				return err
			}, time.Second*5, time.Millisecond*500)
			require.NoError(t, err)

			for _, replicaSet := range replicaSetList.Items {
				// Assert that our expected annotations are in there. In particular we explicitly allow other
				// annotations to be added beyond the ones we asked for. So a direct comparison of the underlying
				// map[string]string objects is inappropriate.
				for expectedName, expectedValue := range expectedAnnotations {
					foundAnnotation := false
					for actualName, actualValue := range replicaSet.Spec.Template.Annotations {
						if expectedName == actualName {
							foundAnnotation = true
							require.Equal(t, expectedValue, actualValue, "Annotation value has been changed")
							break
						}
					}
					if !foundAnnotation {
						t.Errorf("Could not find annotation %s on ReplicaSet %s's pod spec", expectedName, replicaSet.Name)
					}
				}
			}
		})

	})

	t.Run("PodResources", func(t *testing.T) {
		// Mock out the etcd API with one that always fails - i.e., we're always in 'bootstrap' mode
		teardownFunc, namespace := s.setupTest(t, &AlwaysFailEtcdAPI{})
		defer teardownFunc()

		etcdCluster := test.ExampleEtcdCluster(namespace)
		etcdCluster.Spec.Replicas = pointer.Int32Ptr(1)

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		etcdCluster.Default()

		t.Run("AppliesResourcesToPod", func(t *testing.T) {
			replicaSetList := &appsv1.ReplicaSetList{}
			err = try.Eventually(func() error {
				err := s.k8sClient.List(s.ctx, replicaSetList, client.InNamespace(namespace))
				if len(replicaSetList.Items) != int(*etcdCluster.Spec.Replicas) {
					return errors.New(fmt.Sprintf("Wrong number of etcd Replica Sets. Had %d wanted %d", len(replicaSetList.Items), int(*etcdCluster.Spec.Replicas)))
				}
				return err
			}, time.Second*5, time.Millisecond*500)
			require.NoError(t, err)

			r1 := replicaSetList.Items[0]
			assert.Equal(t, *etcdCluster.Spec.PodTemplate.Resources, r1.Spec.Template.Spec.Containers[0].Resources)
		})
	})

	t.Run("PodAffinity", func(t *testing.T) {
		teardownFunc, namespace := s.setupTest(t, &AlwaysFailEtcdAPI{})
		defer teardownFunc()

		etcdCluster := test.ExampleEtcdCluster(namespace)

		expectedAffinity := &v1.Affinity{
			PodAntiAffinity: &v1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
					{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								clusterLabel: "foo",
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
				PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
					{
						Weight: 100,
						PodAffinityTerm: v1.PodAffinityTerm{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									clusterLabel: "foo",
								},
							},
							TopologyKey: "failure-domain.beta.kubernetes.io/zone",
						},
					},
				},
			},
		}
		etcdCluster.Spec.PodTemplate.Affinity = expectedAffinity

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		etcdCluster.Default()

		t.Run("AppliesAffinityToPod", func(t *testing.T) {
			replicaSetList := &appsv1.ReplicaSetList{}
			err = try.Eventually(func() error {
				err := s.k8sClient.List(s.ctx, replicaSetList, client.InNamespace(namespace))
				if len(replicaSetList.Items) != int(*etcdCluster.Spec.Replicas) {
					return errors.New(fmt.Sprintf("Wrong number of etcd Replica Sets. Had %d wanted %d", len(replicaSetList.Items), int(*etcdCluster.Spec.Replicas)))
				}
				return err
			}, time.Second*5, time.Millisecond*500)
			require.NoError(t, err)

			for _, replicaSet := range replicaSetList.Items {
				assert.Equal(t, expectedAffinity, replicaSet.Spec.Template.Spec.Affinity)
			}
		})
	})

	t.Run("StatusVersion", func(t *testing.T) {
		etcdAPI := &WrapperEtcdAPI{wrapped: &AlwaysFailEtcdAPI{}}

		teardownFunc, namespace := s.setupTest(t, etcdAPI)
		defer teardownFunc()

		etcdCluster := test.ExampleEtcdCluster(namespace)

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		etcdCluster.Default()

		etcdClusterKey, err := client.ObjectKeyFromObject(etcdCluster)
		require.NoError(t, err)

		t.Run("ReportsEmptyVersion", func(t *testing.T) {
			etcdAPI.Wrap(&AlwaysFailEtcdAPI{})

			err = try.Eventually(func() error {
				var fetchedCluster etcdv1alpha1.EtcdCluster
				err := s.k8sClient.Get(s.ctx, etcdClusterKey, &fetchedCluster)
				require.NoError(t, err)

				defer func() {
					err = s.triggerReconcile(&fetchedCluster)
					require.NoError(t, err)
				}()

				const expectedVersion = ""
				actualVersion := fetchedCluster.Status.ClusterVersion
				if expectedVersion != actualVersion {
					return fmt.Errorf(
						"unexpected Status.ClusterVersion. Wanted: %s, Got: %s",
						expectedVersion, actualVersion,
					)
				}
				return nil
			}, time.Second*5, time.Second)
			require.NoError(t, err)
		})

		t.Run("ReportsExpectedVersion", func(t *testing.T) {
			expectedVersion := semver.Must(semver.NewVersion("3.2.0"))
			staticAPI := fakeEtcdForEtcdCluster(*etcdCluster)
			staticAPI.ClusterVersion = expectedVersion.String()
			etcdAPI.Wrap(staticAPI)

			err = try.Eventually(func() error {
				var fetchedCluster etcdv1alpha1.EtcdCluster
				err := s.k8sClient.Get(s.ctx, etcdClusterKey, &fetchedCluster)
				require.NoError(t, err)

				defer func() {
					err = s.triggerReconcile(&fetchedCluster)
					require.NoError(t, err)
				}()

				if fetchedCluster.Status.ClusterVersion == "" {
					return fmt.Errorf("Status.ClusterVersion was empty")
				}
				actualVersion, err := semver.NewVersion(fetchedCluster.Status.ClusterVersion)
				require.NoError(t, err)
				if !expectedVersion.Equal(*actualVersion) {
					return fmt.Errorf(
						"unexpected Status.ClusterVersion. Wanted: %s, Got: %s",
						expectedVersion, actualVersion,
					)
				}
				return nil
			}, time.Second*5, time.Second)
			require.NoError(t, err)
		})
	})

	t.Run("VersionUpgrade", func(t *testing.T) {
		etcdAPI := &WrapperEtcdAPI{wrapped: &AlwaysFailEtcdAPI{}}

		teardownFunc, namespace := s.setupTest(t, etcdAPI)
		defer teardownFunc()

		const expectedReplicas = 3
		const originalVersion = "3.0.0"
		const newVersion = "3.0.1"

		t.Log("Given a 3-node cluster at version:", originalVersion)

		etcdCluster := test.ExampleEtcdCluster(namespace)
		etcdCluster.Spec.Replicas = pointer.Int32Ptr(expectedReplicas)
		etcdCluster.Spec.Version = originalVersion

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		etcdCluster.Default()

		etcdClusterKey, err := client.ObjectKeyFromObject(etcdCluster)
		require.NoError(t, err)

		err = try.Eventually(func() error {
			var fetchedCluster etcdv1alpha1.EtcdCluster
			err := s.k8sClient.Get(s.ctx, etcdClusterKey, &fetchedCluster)
			require.NoError(t, err)

			if fetchedCluster.Status.Replicas != expectedReplicas {
				return fmt.Errorf("wrong number of peers")
			}
			return nil
		}, time.Second*5, time.Second)
		require.NoError(t, err)

		staticAPI := fakeEtcdForEtcdCluster(*etcdCluster)
		etcdAPI.Wrap(staticAPI)

		err = try.Eventually(func() error {
			var fetchedCluster etcdv1alpha1.EtcdCluster
			err := s.k8sClient.Get(s.ctx, etcdClusterKey, &fetchedCluster)
			require.NoError(t, err)

			defer func() {
				err := s.triggerReconcile(&fetchedCluster)
				require.NoError(t, err)
			}()

			if len(fetchedCluster.Status.Members) != expectedReplicas {
				return fmt.Errorf("wrong number of members")
			}
			return nil
		}, time.Second*5, time.Second)
		require.NoError(t, err)

		t.Log("Changing EtcdCluster.Spec.Version to a newer version", newVersion)
		updated := etcdCluster.DeepCopy()
		updated.Spec.Version = newVersion
		err = s.k8sClient.Patch(s.ctx, updated, client.MergeFrom(etcdCluster))
		require.NoError(t, err)

		t.Log("Causes Peers to be recreated in reverse name order")
		staticAPI.ServerVersion = newVersion

		t.Log("Causes all the EtcdPeers to be updated to that new version", newVersion)
		err = try.Eventually(func() error {
			var peers etcdv1alpha1.EtcdPeerList
			err := s.k8sClient.List(s.ctx, &peers, client.InNamespace(namespace))
			require.NoError(t, err)
			for _, peer := range peers.Items {
				if peer.Spec.Version != newVersion {
					return fmt.Errorf("peer %q had unexpected version %q", peer.Name, peer.Spec.Version)
				}
			}
			return nil
		}, time.Second*5, time.Second)
		require.NoError(t, err)

		// t.Run("ReportsExpectedVersion", func(t *testing.T) {
		//	expectedVersion := semver.Must(semver.NewVersion("3.2.0"))
		//	staticAPI := fakeEtcdForEtcdCluster(*etcdCluster)
		//	staticAPI.ClusterVersion = expectedVersion.String()
		//	etcdAPI.Wrap(staticAPI)

		// })
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

func expectedStatusForCluster(c etcdv1alpha1.EtcdCluster) etcdv1alpha1.EtcdClusterStatus {
	members := make([]etcdv1alpha1.EtcdMember, *c.Spec.Replicas)
	for i := range members {
		members[i] = etcdv1alpha1.EtcdMember{
			ID:   fmt.Sprintf("SOMEID%d", i),
			Name: fmt.Sprintf("%s-%d", c.Name, i),
		}
	}
	return etcdv1alpha1.EtcdClusterStatus{
		Replicas:       *c.Spec.Replicas,
		Members:        members,
		ClusterVersion: c.Spec.Version,
	}
}

func expectedEtcdMembersForCluster(c etcdv1alpha1.EtcdCluster) []etcdclient.Member {
	members := make([]etcdclient.Member, *c.Spec.Replicas)
	for i := range members {
		name := fmt.Sprintf("%s-%d", c.Name, i)
		members[i] = etcdclient.Member{
			ID:         fmt.Sprintf("SOMEID%d", i),
			Name:       name,
			PeerURLs:   []string{fmt.Sprintf("http://%s.%s.%s.svc:2380", name, c.Name, c.Namespace)},
			ClientURLs: []string{fmt.Sprintf("http://%s.%s.%s.svc:2379", name, c.Name, c.Namespace)},
		}
	}
	return members
}

func expectedEtcdPeersForCluster(c etcdv1alpha1.EtcdCluster) etcdv1alpha1.EtcdPeerList {
	items := make([]etcdv1alpha1.EtcdPeer, *c.Spec.Replicas)
	for i := range items {
		name := fmt.Sprintf("%s-%d", c.Name, i)
		items[i] = etcdv1alpha1.EtcdPeer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: c.Namespace,
			},
		}
	}
	return etcdv1alpha1.EtcdPeerList{
		Items: items,
	}
}

func listOfRuntimeObjects(list metav1.ListInterface) []runtime.Object {
	switch l := list.(type) {
	case *etcdv1alpha1.EtcdPeerList:
		ol := make([]runtime.Object, len(l.Items))
		for i := range l.Items {
			ol[i] = &l.Items[i]
		}
		return ol
	case *v1.PersistentVolumeClaimList:
		ol := make([]runtime.Object, len(l.Items))
		for i := range l.Items {
			ol[i] = &l.Items[i]
		}
		return ol
	default:
		panic("unknown type")
	}

}

func setOfNamespacedNamesForList(list []runtime.Object) sets.String {
	names := sets.NewString()
	for _, o := range list {
		nn, err := client.ObjectKeyFromObject(o)
		if err != nil {
			panic(err)
		}
		names.Insert(nn.String())
	}
	return names
}

func hasIdenticalListItemNames(expected, actual metav1.ListInterface) error {
	expectedObjects := listOfRuntimeObjects(expected)
	expectedNames := setOfNamespacedNamesForList(expectedObjects)
	actualObjects := listOfRuntimeObjects(actual)
	actualNames := setOfNamespacedNamesForList(actualObjects)
	if diff := cmp.Diff(expectedNames, actualNames); diff != "" {
		return fmt.Errorf("unexpected items: diff --- expected, +++ actual\n%s", diff)
	}
	return nil
}

func TestPeerForCluster(t *testing.T) {
	cluster := test.ExampleEtcdCluster("ns1")
	peer := peerForCluster(cluster, "peer-1")

	expectations := map[string]interface{}{
		`.spec.version`: cluster.Spec.Version,
	}
	test.AssertStructFields(t, expectations, peer)
}
