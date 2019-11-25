package controllers

import (
	"testing"
	"time"

	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *controllerSuite) testBackupScheduleController(t *testing.T) {
	teardownFunc, namespace := s.setupTest(t)
	defer teardownFunc()

	// Create the backup schedule.
	backupSchedule := test.ExampleEtcdBackupSchedule(namespace)
	err := s.k8sClient.Create(s.ctx, backupSchedule)
	require.NoError(t, err)

	// Check that it creates a EtcdBackup resource.
	var pods *corev1.PodList
	err = try.Eventually(func() error {
		return s.k8sClient.List(s.ctx, pods, client.MatchingLabels{
			scheduleLabel: backupSchedule.Name,
		})
	}, time.Second*5, time.Minute)
	require.NoError(t, err)
	require.Len(t, pods.Items, 1)
}
