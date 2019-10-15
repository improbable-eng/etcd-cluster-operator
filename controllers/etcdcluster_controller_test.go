package controllers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

func (s *controllerSuite) testClusterController(t *testing.T) {
	t.Run("TestClusterController_OnCreation_CreatesReplicaSet", func(t *testing.T) {
		teardownFunc := s.setupTest(t)
		defer teardownFunc()

		etcdCluster := &etcdv1alpha1.EtcdCluster{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
				Name:        "bees",
				Namespace:   "default",
			},
			Spec: etcdv1alpha1.EtcdClusterSpec{
				Replicas: 3,
			},
		}

		err := s.k8sClient.Create(s.ctx, etcdCluster)
		require.NoError(t, err, "failed to create EtcdCluster resource")

		peers := &etcdv1alpha1.EtcdPeerList{}

		err = try.Eventually(func() error {
			return s.k8sClient.List(s.ctx, peers)
		}, time.Second*5, time.Millisecond*500)
		require.NoError(t, err)
	})
}
