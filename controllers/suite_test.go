package controllers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/stretchr/testify/require"
	etcdclient "go.etcd.io/etcd/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/crontest"
)

type controllerSuite struct {
	ctx       context.Context
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	etcd      etcd.EtcdAPI
}

func setupSuite(t *testing.T) (suite *controllerSuite, teardownFunc func()) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Minute*5)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
		KubeAPIServerFlags: append(
			envtest.DefaultKubeAPIServerFlags,
			"--advertise-address=127.0.0.1",
		),
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	err = etcdv1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	// Add new resources here.

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	require.NoError(t, err)
	require.NotNil(t, k8sClient)

	stopFunc := func() {
		err := suite.testEnv.Stop()
		require.NoError(t, err)

		ctxCancel()
	}

	return &controllerSuite{
		ctx:       ctx,
		cfg:       cfg,
		k8sClient: k8sClient,
		testEnv:   testEnv,
		etcd:      nil,
	}, stopFunc
}

// MembershipAPI exists so we can 'forward' the method on the EtcdAPI interface to our internal etcd. This is so that we
// can pass ourselves in as an implementation of an EtcdAPI at test assembly time, but a test can switch out it's
// internal implementation in the middle of a test by setting `controllerSuite.etcd`
func (s *controllerSuite) MembershipAPI(config etcdclient.Config) (etcdclient.MembersAPI, error) {
	return s.etcd.MembershipAPI(config)
}

func (s *controllerSuite) setupTest(t *testing.T) (teardownFunc func(), namespaceName string) {
	stopCh := make(chan struct{})
	logger := test.TestLogger{
		T: t,
	}
	// This allows us to see controller-runtime logs in the test results.
	// E.g. controller-runtime will log any error that we return from the Reconcile function,
	// so this saves us having to log those errors again, inside Reconcile.
	ctrl.SetLogger(logger)

	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: petname.Generate(2, "-"),
		},
	}
	err := s.k8sClient.Create(s.ctx, namespace)
	require.NoError(t, err)

	mgr, err := ctrl.NewManager(s.cfg, ctrl.Options{
		Namespace: namespace.Name,
	})
	require.NoError(t, err, "failed to create manager")

	peerController := EtcdPeerReconciler{
		Client: mgr.GetClient(),
		Log:    logger.WithName("EtcdPeer"),
	}
	err = peerController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to set up EtcdPeer controller")

	clusterController := EtcdClusterReconciler{
		Client:   mgr.GetClient(),
		Log:      logger.WithName("EtcdCluster"),
		Recorder: mgr.GetEventRecorderFor("etcdcluster-reconciler"),
		Etcd:     s,
	}
	err = clusterController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to setup EtcdCluster controller")

	backupController := EtcdBackupScheduleReconciler{
		Client:      mgr.GetClient(),
		Log:         logger.WithName("EtcdCluster"),
		CronHandler: crontest.FakeCron{},
		Schedules:   NewScheduleMap(),
	}
	err = backupController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to setup EtcdBackupSchedule controller")

	restoreController := EtcdRestoreReconciler{
		Client: mgr.GetClient(),
		Log:    logger.WithName("EtcdCluster"),
	}
	err = restoreController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to setup EtcdRestoreReconciler controller")

	go func() {
		err := mgr.Start(stopCh)
		require.NoError(t, err, "failed to start manager")
	}()

	return func() {
		err := s.k8sClient.Delete(s.ctx, namespace)
		require.NoErrorf(t, err, "Failed to delete test namespace: %#v", namespace)
		close(stopCh)
	}, namespace.Name
}

func TestAPIs(t *testing.T) {
	suite, teardownFunc := setupSuite(t)
	defer teardownFunc()

	t.Run("PeerControllers", suite.testPeerController)
	t.Run("ClusterControllers", suite.testClusterController)
	t.Run("BackupScheduleControllers", suite.testBackupScheduleController)
}
