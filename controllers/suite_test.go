package controllers

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	logtest "github.com/go-logr/logr/testing"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

type controllerSuite struct {
	ctx       context.Context
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
}

func setupSuite(t *testing.T) (suite *controllerSuite, teardownFunc func()) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*30)

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

func (s *controllerSuite) setupTest(t *testing.T) (teardownFunc func(), namespaceName string) {
	stopCh := make(chan struct{})

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
		Log: logtest.TestLogger{
			T: t,
		},
	}
	err = peerController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to set up EtcdPeer controller")

	clusterController := EtcdClusterReconciler{
		Client: mgr.GetClient(),
		Log: logtest.TestLogger{
			T: t,
		},
	}
	err = clusterController.SetupWithManager(mgr)
	require.NoError(t, err, "failed to setup EtcdCluster controller")

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
}
