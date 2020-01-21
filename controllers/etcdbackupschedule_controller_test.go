package controllers

import (
	"strings"
	"testing"
	"time"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *controllerSuite) testBackupScheduleController(t *testing.T) {
	teardownFunc, namespace := s.setupTest(t, &AlwaysFailEtcdAPI{})
	defer teardownFunc()

	backupSchedule := test.ExampleEtcdBackupSchedule(namespace)

	t.Run("CreatesBackupResource", func(t *testing.T) {
		// Create the backup schedule.
		err := s.k8sClient.Create(s.ctx, backupSchedule)
		require.NoError(t, err)

		// Check that it creates a EtcdBackup resource.
		backups := &etcdv1alpha1.EtcdBackupList{}
		err = try.Eventually(func() error {
			return s.k8sClient.List(s.ctx, backups, &client.MatchingLabels{
				scheduleLabel: backupSchedule.Name,
			}, &client.ListOptions{
				Namespace: namespace,
			})
		}, time.Second*30, time.Second)
		require.NoError(t, err)
		require.Len(t, backups.Items, 1)
		require.True(t, strings.HasPrefix(backups.Items[0].Name, backupSchedule.Name), "EtcdBackup should have the EtcdSchedule name as a prefix")
	})
}
