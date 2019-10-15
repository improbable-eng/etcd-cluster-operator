package controllers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

// requireEnvVar finds an environment variable and returns its value, or fails
func requireEnvVar(t *testing.T, env []corev1.EnvVar, evName string) string {
	for _, ev := range env {
		if ev.Name == evName {
			return ev.Value
		}
	}
	require.Fail(t, fmt.Sprintf("%s environment variable unset", evName))
	return ""
}

func (s *controllerSuite) testPeerController(t *testing.T) {
	t.Run("TestPeerController_OnCreation_CreatesReplicaSet", func(t *testing.T) {
		teardownFunc := s.setupTest(t)
		defer teardownFunc()

		etcdPeer := &etcdv1alpha1.EtcdPeer{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
				Name:        "bees",
				Namespace:   "default",
			},
			Spec: etcdv1alpha1.EtcdPeerSpec{
				ClusterName: "my-cluster",
				Bootstrap: &etcdv1alpha1.Bootstrap{
					Static: &etcdv1alpha1.StaticBootstrap{
						InitialCluster: []etcdv1alpha1.InitialClusterMember{
							{
								Name: "bees",
								Host: "bees.my-cluster.default.svc",
							},
							{
								Name: "magic",
								Host: "magic.my-cluster.default.svc",
							},
							{
								Name: "goose",
								Host: "goose.my-cluster.default.svc",
							},
						},
					},
				},
			},
		}

		err := s.k8sClient.Create(s.ctx, etcdPeer)
		require.NoError(t, err, "failed to create EtcdPeer resource")

		replicaSet := &appsv1.ReplicaSet{}

		err = try.Eventually(func() error {
			return s.k8sClient.Get(s.ctx, client.ObjectKey{
				// Same name and namespace as the EtcdPeer above
				Name:      etcdPeer.Name,
				Namespace: etcdPeer.Namespace,
			}, replicaSet)
		}, time.Second*5, time.Millisecond*500)
		require.NoError(t, err)

		require.Equal(t, int32(1), *replicaSet.Spec.Replicas, "Number of replicas was not one")
		require.Equal(t,
			replicaSet.Spec.Template.Spec.Hostname,
			etcdPeer.Name,
			"Peer name not set as Pod hostname")
		require.Equal(t,
			replicaSet.Spec.Template.Spec.Subdomain,
			etcdPeer.Spec.ClusterName,
			"Cluster name not set as Pod subdomain")

		// Find the etcd container
		var etcdContainer corev1.Container
		for _, container := range replicaSet.Spec.Template.Spec.Containers {
			if container.Name == "etcd" {
				etcdContainer = container
				break
			}
		}
		require.NotNil(t, etcdContainer, "Failed to find an etcd container")

		image := strings.Split(etcdContainer.Image, ":")[0]
		require.Equal(t, "quay.io/coreos/etcd", image, "etcd Image was not the CoreOS one")

		peers := strings.Split(requireEnvVar(t, etcdContainer.Env, "ETCD_INITIAL_CLUSTER"), ",")
		require.Len(t, peers, 3)
		require.Contains(t, peers, "bees=http://bees.my-cluster.default.svc:2380")
		require.Contains(t, peers, "goose=http://goose.my-cluster.default.svc:2380")
		require.Contains(t, peers, "magic=http://magic.my-cluster.default.svc:2380")

		require.Equal(t,
			etcdPeer.Name,
			requireEnvVar(t, etcdContainer.Env, "ETCD_NAME"),
			"ETCD_NAME environment variable set incorrectly",
		)

		require.Equal(t,
			fmt.Sprintf("http://%s.%s.%s.svc:2380",
				etcdPeer.Name,
				etcdPeer.Spec.ClusterName,
				etcdPeer.Namespace),
			requireEnvVar(t, etcdContainer.Env, "ETCD_INITIAL_ADVERTISE_PEER_URLS"),
			"ETCD_INITIAL_ADVERTISE_PEER_URLS environment variable set incorrectly",
		)

		require.Equal(t,
			fmt.Sprintf("http://%s.%s.%s.svc:2379",
				etcdPeer.Name,
				etcdPeer.Spec.ClusterName,
				etcdPeer.Namespace),
			requireEnvVar(t, etcdContainer.Env, "ETCD_ADVERTISE_CLIENT_URLS"),
			"ETCD_ADVERTISE_CLIENT_URLS environment variable set incorrectly",
		)

		require.Equal(t,
			"http://0.0.0.0:2380",
			requireEnvVar(t, etcdContainer.Env, "ETCD_LISTEN_PEER_URLS"),
			"ETCD_LISTEN_PEER_URLS environment variable set incorrectly",
		)

		require.Equal(t,
			"http://0.0.0.0:2379",
			requireEnvVar(t, etcdContainer.Env, "ETCD_LISTEN_CLIENT_URLS"),
			"ETCD_LISTEN_CLIENT_URLS environment variable set incorrectly",
		)

		require.Equal(t,
			"new",
			requireEnvVar(t, etcdContainer.Env, "ETCD_INITIAL_CLUSTER_STATE"),
			"ETCD_INITIAL_CLUSTER_STATE environment variable set incorrectly",
		)
	})
}
