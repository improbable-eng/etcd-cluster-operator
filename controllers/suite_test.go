package controllers

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
}

func setupSuite(t *testing.T) (suite *controllerSuite, teardownFunc func()) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Minute*5)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "config", "crd", "bases")},
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
	}, stopFunc
}

func (s *controllerSuite) setupTest(t *testing.T, etcdAPI etcd.APIBuilder) (teardownFunc func(), namespaceName string) {
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
		Scheme:    scheme.Scheme,
	})
	require.NoError(t, err, "failed to create manager")

	peerController := EtcdPeerReconciler{
		Client:         mgr.GetClient(),
		Log:            logger.WithName("EtcdPeer"),
		Etcd:           etcdAPI,
		EtcdRepository: "quay.io/coreos/etcd",
	}
	err = peerController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to set up EtcdPeer controller")

	clusterController := EtcdClusterReconciler{
		Client:   mgr.GetClient(),
		Log:      logger.WithName("EtcdCluster"),
		Recorder: mgr.GetEventRecorderFor("etcdcluster-reconciler"),
		Etcd:     etcdAPI,
	}
	err = clusterController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to setup EtcdCluster controller")

	backupController := EtcdBackupReconciler{
		Client:           mgr.GetClient(),
		Log:              logger.WithName("EtcdBackup"),
		Scheme:           mgr.GetScheme(),
		BackupAgentImage: "UNTESTED",
		ProxyURL:         "UNTESTED",
		Recorder:         mgr.GetEventRecorderFor("etcdbackup-reconciler"),
	}
	err = backupController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to setup EtcdBackup controller")

	backupScheduleController := EtcdBackupScheduleReconciler{
		Client:      mgr.GetClient(),
		Log:         logger.WithName("EtcdBackupSchedule"),
		CronHandler: crontest.FakeCron{},
		Schedules:   NewScheduleMap(),
	}
	err = backupScheduleController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to setup EtcdBackupSchedule controller")

	restoreController := EtcdRestoreReconciler{
		Client:   mgr.GetClient(),
		Recorder: mgr.GetEventRecorderFor("etcdrestore-reconciler"),
		Log:      logger.WithName("EtcdRestore"),
	}
	err = restoreController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to setup EtcdRestoreReconciler controller")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := mgr.Start(stopCh)
		require.NoError(t, err, "failed to start manager")
	}()

	return func() {
		defer func() {
			close(stopCh)
			wg.Wait()
		}()
		err := s.k8sClient.Delete(s.ctx, namespace)
		require.NoErrorf(t, err, "Failed to delete test namespace: %#v", namespace)
	}, namespace.Name
}

// triggerReconcile forces a Reconcile by making a trivial change to the annotations
// of the supplied object.
// A work around for https://github.com/improbable-eng/etcd-cluster-operator/issues/76
func (s *controllerSuite) triggerReconcile(obj runtime.Object) error {
	m := meta.NewAccessor()
	updated := obj.DeepCopyObject()
	annotations, err := m.Annotations(updated)
	if err != nil {
		return err
	}
	if annotations == nil {
		annotations = map[string]string{}
	}
	value, found := annotations["last-trigger-value"]
	if !found {
		value = "0"
	}
	index, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	value = strconv.Itoa(index + 1)
	annotations["last-trigger-value"] = value
	err = m.SetAnnotations(updated, annotations)
	if err != nil {
		return err
	}
	return s.k8sClient.Patch(s.ctx, updated, client.MergeFrom(obj))
}

func TestAPIs(t *testing.T) {
	suite, teardownFunc := setupSuite(t)
	defer teardownFunc()

	t.Run("PeerControllers", suite.testPeerController)
	t.Run("ClusterControllers", suite.testClusterController)
	t.Run("BackupScheduleControllers", suite.testBackupScheduleController)
}
