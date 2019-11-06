package e2e

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	etcd "go.etcd.io/etcd/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	kindv1alpha3 "sigs.k8s.io/kind/pkg/apis/config/v1alpha3"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/create"
	"sigs.k8s.io/kind/pkg/container/cri"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

const (
	expectedClusterSize = 3
)

var (
	fUseKind           = flag.Bool("kind", false, "Creates a Kind cluster to run the tests against.")
	fUseCurrentContext = flag.Bool("current-context", false, "Runs the tests against the current Kubernetes context, the path to kube config defaults to ~/.kube/config, unless overridden by the KUBECONFIG environment variable.")
	fRepoRoot          = flag.String("repo-root", "", "The absolute path to the root of the etcd-cluster-operator git repository.")
	fCleanup           = flag.Bool("cleanup", true, "Tears down the Kind cluster once the test is finished.")

	etcdConfig = etcd.Config{
		Endpoints: []string{"http://127.0.0.1:2379"},
		Transport: etcd.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
)

func objFromYaml(objBytes []byte) (runtime.Object, error) {
	scheme := runtime.NewScheme()
	if err := etcdv1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return runtime.Decode(
		serializer.NewCodecFactory(scheme).UniversalDeserializer(),
		objBytes,
	)
}

func objFromYamlPath(objPath string) (runtime.Object, error) {
	objBytes, err := ioutil.ReadFile(objPath)
	if err != nil {
		return nil, err
	}
	return objFromYaml(objBytes)
}

func getSpec(t *testing.T, o interface{}) interface{} {
	switch ot := o.(type) {
	case *etcdv1alpha1.EtcdCluster:
		return ot.Spec
	case *etcdv1alpha1.EtcdPeer:
		return ot.Spec
	default:
		require.Failf(t, "unknown type", "%#v", o)
	}
	return nil
}

func TestE2E_Kind(t *testing.T) {
	if !*fUseKind {
		t.Skip()
	}

	// Tag for running this test, for naming resources.
	operatorImage := "etcd-cluster-operator:test"

	// Create Kind cluster to run the workloads.
	kind, stopKind := setupLocalCluster(t)
	defer stopKind()

	// Ensure Kind gets stopped on SIGINT/SIGTERM.
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		stopKind()
	}()

	kubectl := &kubectlContext{
		t:          t,
		configPath: kind.KubeConfigPath(),
	}

	t.Log("Installing cert-manager")
	err := kubectl.Apply("--validate=false", "--filename=https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml")
	require.NoError(t, err)

	t.Log("Waiting for cert-manager to be ready")
	err = kubectl.Wait("--for=condition=Available", "--timeout=300s", "apiservice", "v1beta1.webhook.cert-manager.io")
	require.NoError(t, err)

	// Ensure CRDs exist in the cluster.
	t.Log("Applying CRDs")
	err = kubectl.Apply("--kustomize", filepath.Join(*fRepoRoot, "config", "crd"))
	require.NoError(t, err)

	// Build the operator.
	t.Log("Building operator image")
	out, err := exec.Command("docker", "build", "-t", operatorImage, *fRepoRoot).CombinedOutput()
	require.NoError(t, err, string(out))

	// Bundle the image to a tar.
	tmpDir, err := ioutil.TempDir("", "etcd-cluster-operator-e2e-test")
	require.NoError(t, err)
	imageTar := filepath.Join(tmpDir, "etcd-cluster-operator.tar")

	out, err = exec.Command("docker", "save", "-o", imageTar, operatorImage).CombinedOutput()
	require.NoError(t, err, string(out))
	imageFile, err := os.Open(imageTar)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, imageFile.Close(), "failed to close operator image tar")
	}()

	// Load the built image into the Kind cluster.
	t.Log("Loading image in to Kind cluster")
	nodes, err := kind.ListInternalNodes()
	require.NoError(t, err)
	for _, node := range nodes {
		err := node.LoadImageArchive(imageFile)
		require.NoError(t, err)
	}

	// Deploy the operator.
	t.Log("Applying operator")
	err = kubectl.Apply("--kustomize", filepath.Join(*fRepoRoot, "config", "test"))
	require.NoError(t, err)

	// Ensure the operator starts.
	err = try.Eventually(func() error {
		out, err := kubectl.Get("--namespace", "eco-system", "deploy", "eco-controller-manager", "-o=jsonpath='{.status.readyReplicas}'")
		if err != nil {
			return err
		}
		if out != "'1'" {
			return errors.New("expected exactly 1 replica of the operator to be available, got: " + out)
		}
		return nil
	}, time.Minute, time.Second*5)
	require.NoError(t, err)

	t.Log("Running tests")
	runAllTests(t, kubectl)
}

func TestE2E_CurrentContext(t *testing.T) {
	if !*fUseCurrentContext {
		t.Skip()
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	configPath := filepath.Join(home, ".kube", "config")
	if path, found := os.LookupEnv("KUBECONFIG"); found {
		configPath = path
	}

	kubectl := &kubectlContext{
		t:          t,
		configPath: configPath,
	}

	runAllTests(t, kubectl)
}

// Starts a Kind cluster on the local machine, exposing port 2379 accepting ETCD connections.
func setupLocalCluster(t *testing.T) (*cluster.Context, func()) {
	t.Log("Starting Kind cluster")
	kind := cluster.NewContext("etcd-e2e")

	err := kind.Create(create.WithV1Alpha3(&kindv1alpha3.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "kind.sigs.k8s.io/v1alpha3",
		},
		Nodes: []kindv1alpha3.Node{
			{
				Role: "control-plane",
				ExtraPortMappings: []cri.PortMapping{
					{
						ContainerPort: 32379,
						HostPort:      2379,
					},
				},
			},
		},
	}))
	require.NoError(t, err)

	tearDown := func() {
		if !*fCleanup {
			return
		}
		t.Log("Stopping Kind cluster")
		err := kind.Delete()
		assert.NoError(t, err, "failed to stop Kind cluster")
	}

	return kind, tearDown
}

func runAllTests(t *testing.T, kubectl *kubectlContext) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	// Deploy a service to expose the cluster to the host machine.
	err := kubectl.Apply(
		"--filename", filepath.Join(*fRepoRoot, "internal", "test", "e2e", "fixtures", "cluster-client-service.yaml"),
	)
	require.NoError(t, err)

	sampleClusterPath := filepath.Join(*fRepoRoot, "config", "samples", "etcd_v1alpha1_etcdcluster.yaml")

	// Deploy the cluster custom resources.
	// Retry for 5 seconds, because the Etcd mutating and validating webhook
	// service may not immediately be responding.
	err = try.Eventually(func() error {
		return kubectl.Apply("--filename", sampleClusterPath)
	}, time.Second*5, time.Second*1)
	require.NoError(t, err)

	t.Run("Defaulting", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			path string
		}{
			{
				name: "EtcdCluster",
				path: "defaulting/etcdcluster.yaml",
			},
			{
				name: "EtcdPeer",
				path: "defaulting/etcdpeer.yaml",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// local object
				lPath := filepath.Join(*fRepoRoot, "config/test/e2e", tc.path)
				l, err := objFromYamlPath(lPath)
				require.NoError(t, err)

				rBytes, err := kubectl.DryRun(lPath)
				require.NoError(t, err, rBytes)

				// Remote object
				r, err := objFromYaml([]byte(rBytes))
				require.NoError(t, err)

				if diff := cmp.Diff(getSpec(t, l), getSpec(t, r)); diff == "" {
					assert.Failf(t, "defaults were not applied to: %s", lPath)
				} else {
					t.Log(diff)
					assert.Contains(t, diff, `AccessModes:`)
					assert.Contains(t, diff, `VolumeMode:`)
				}
			})

		}
	})

	t.Run("Validation", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			path string
		}{
			{
				name: "EtcdCluster",
				path: "validation/etcdcluster_missing_storageclassname.yaml",
			},
			{
				name: "EtcdPeer",
				path: "validation/etcdpeer_missing_storageclassname.yaml",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				lPath := filepath.Join(*fRepoRoot, "config/test/e2e", tc.path)
				errorMessage, err := kubectl.DryRun(lPath)
				require.Error(t, err)
				assert.Regexp(
					t,
					`^Error from server \(spec.storage.volumeClaimTemplate.storageClassName: Required value\):`,
					errorMessage,
				)
			})
		}
	})

	t.Run("EtcdIsReachable", func(t *testing.T) {
		etcdClient, err := etcd.New(etcdConfig)
		require.NoError(t, err)

		err = try.Eventually(func() error {
			t.Log("Checking if ETCD is available")
			members, err := etcd.NewMembersAPI(etcdClient).List(ctx)
			if err != nil {
				return err
			}

			if len(members) != expectedClusterSize {
				return errors.New(fmt.Sprintf("expected %d etcd peers, got %d", expectedClusterSize, len(members)))
			}
			return nil
		}, time.Minute*2, time.Second*10)
		require.NoError(t, err)
		t.Log("ETCD is reachable from host machine")
	})

	t.Run("StatusUpdate", func(t *testing.T) {
		err = try.Eventually(func() error {
			t.Log("Checking if the EtcdCluster resource has the correct status")
			members, err := kubectl.Get("--namespace", "default", "etcdcluster", "my-cluster", "-o=jsonpath='{.status.members...name}'")

			if err != nil {
				return err
			}
			// Don't assert on exact memebers, just that we have three of them.
			if len(strings.Split(members, " ")) != 3 {
				return errors.New(fmt.Sprintf("Expected etcd member list to have three members. Had %d.", len(members)))
			}
			return nil
		}, time.Minute*2, time.Second*10)
		require.NoError(t, err)
	})
}
