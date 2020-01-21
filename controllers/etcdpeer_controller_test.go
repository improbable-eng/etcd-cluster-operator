package controllers

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
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

func exampleEtcdPeer(namespace string) *etcdv1alpha1.EtcdPeer {
	return test.ExampleEtcdPeer(namespace)
}

func (s *controllerSuite) testPeerController(t *testing.T) {
	t.Run("TestPeerController_OnCreation_CreatesReplicaSet", func(t *testing.T) {
		teardownFunc, namespace := s.setupTest(t, &AlwaysFailEtcdAPI{})
		defer teardownFunc()

		etcdPeer := exampleEtcdPeer(namespace)
		const cpuLimit = "2.2"
		const expectedGoMaxProcs = "2"
		etcdPeer.Spec.PodTemplate.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(cpuLimit)
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

		expectations := map[string]interface{}{
			`.spec.template.spec.volumes[?(@.name=="etcd-data")].persistentVolumeClaim.claimName`:              etcdPeer.Name,
			`.spec.template.spec.containers[?(@.name=="etcd")].volumeMounts[?(@.name=="etcd-data")].mountPath`: etcdDataMountPath,
			`.spec.template.spec.containers[?(@.name=="etcd")].env[?(@.name=="ETCD_DATA_DIR")].value`:          etcdDataMountPath,
			`.spec.template.spec.containers[?(@.name=="etcd")].env[?(@.name=="GOMAXPROCS")].value`:             expectedGoMaxProcs,
		}
		test.AssertStructFields(t, expectations, replicaSet)

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
		require.Contains(t, peers, fmt.Sprintf("bees=http://bees.my-cluster.%s.svc:2380", namespace))
		require.Contains(t, peers, fmt.Sprintf("goose=http://goose.my-cluster.%s.svc:2380", namespace))
		require.Contains(t, peers, fmt.Sprintf("magic=http://magic.my-cluster.%s.svc:2380", namespace))

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
	t.Run("CreatesPersistentVolumeClaim", func(t *testing.T) {
		teardown, namespace := s.setupTest(t, &AlwaysFailEtcdAPI{})
		defer teardown()
		peer := exampleEtcdPeer(namespace)
		err := s.k8sClient.Create(s.ctx, peer)
		require.NoError(t, err, "failed to create EtcdPeer resource")
		actualPvc := &corev1.PersistentVolumeClaim{}
		err = try.Eventually(
			func() error {
				return s.k8sClient.Get(s.ctx, client.ObjectKey{
					Name:      peer.Name,
					Namespace: peer.Namespace,
				}, actualPvc)
			},
			time.Second*5, time.Millisecond*500,
		)
		require.NoError(t, err, "PVC was not created")

		// Apply defaults here so that our expected object has all the same
		// defaults as those used in the Reconcile function
		peer.Default()

		require.Equal(t, *peer.Spec.Storage.VolumeClaimTemplate, actualPvc.Spec, "Unexpected PVC spec")
	})
}

func TestGoMaxProcs(t *testing.T) {
	tests := map[string]struct {
		limit    string
		expected *int64
	}{
		"Negative": {
			limit:    "-1",
			expected: nil,
		},
		"Zero": {
			limit:    "0",
			expected: nil,
		},
		"JustAboveZero": {
			limit:    "0.1",
			expected: pointer.Int64Ptr(1),
		},
		"PointFive": {
			limit:    "0.5",
			expected: pointer.Int64Ptr(1),
		},
		"AlmostOne": {
			limit:    "0.9",
			expected: pointer.Int64Ptr(1),
		},
		"ExactlyOne": {
			limit:    "1",
			expected: pointer.Int64Ptr(1),
		},
		"JustAboveOne": {
			limit:    "1.1",
			expected: pointer.Int64Ptr(1),
		},
		"OnePointFive": {
			limit:    "1.5",
			expected: pointer.Int64Ptr(1),
		},
		"AlmostTwo": {
			limit:    "1.9",
			expected: pointer.Int64Ptr(1),
		},
		"ExactlyTwo": {
			limit:    "2",
			expected: pointer.Int64Ptr(2),
		},
		"TwoPointFive": {
			limit:    "2.5",
			expected: pointer.Int64Ptr(2),
		},
		"AlmostThree": {
			limit:    "2.9",
			expected: pointer.Int64Ptr(2),
		},
	}

	for title, tc := range tests {
		t.Run(title, func(t *testing.T) {
			actual := goMaxProcs(resource.MustParse(tc.limit))
			if tc.expected == nil {
				assert.Nil(t, actual)
			} else {
				assert.Equal(t, *tc.expected, *actual)
			}
		})
	}

}
