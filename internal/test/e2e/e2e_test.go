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
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	etcd "go.etcd.io/etcd/client"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	kindv1alpha3 "sigs.k8s.io/kind/pkg/apis/config/v1alpha3"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cluster/create"
	"sigs.k8s.io/kind/pkg/container/cri"

	semver "github.com/coreos/go-semver/semver"
	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

const (
	expectedClusterSize = 3
)

var (
	fUseKind           = flag.Bool("kind", false, "Creates a Kind cluster to run the tests against.")
	fOutputDirectory   = flag.String("output-directory", "/tmp/etcd-e2e", "The absolute path to a directory where E2E results logs will be saved.")
	fKindClusterName   = flag.String("kind-cluster-name", "etcd-e2e", "The name of the Kind cluster to use or create")
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

// Starts a Kind cluster on the local machine, exposing port 2379 accepting ETCD connections.
func startKind(t *testing.T, ctx context.Context, stopped chan struct{}) (kind *cluster.Context, err error) {
	clusterExists := true
	// Only attempt to delete the cluster if we created it.
	// But always ensure that the stopped channel gets closed when the context
	// ends or is cancelled.
	go func() {
		defer close(stopped)
		<-ctx.Done()
		if kind == nil {
			return
		}
		t.Log("Collecting Kind logs.")
		err := kind.CollectLogs(path.Join(*fOutputDirectory, "kind"))
		assert.NoError(t, err, "failed to collect Kind logs")
		if !*fCleanup {
			t.Log("Not deleting Kind cluster because --cleanup=false")
			return
		}
		if clusterExists {
			t.Log("Not deleting Kind cluster because this was an existing cluster.")
			return
		}
		t.Log("Deleting Kind cluster.")
		err = kind.Delete()
		assert.NoError(t, err)
	}()

	clusters, err := cluster.List()
	if err != nil {
		return nil, err
	}
	for _, kind := range clusters {
		if kind.Name() == *fKindClusterName {
			t.Log("Found existing Kind cluster")
			return &kind, nil
		}
	}
	clusterExists = false
	t.Log("Starting new Kind cluster")
	kind = cluster.NewContext(*fKindClusterName)
	err = kind.Create(create.WithV1Alpha3(&kindv1alpha3.Cluster{
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
	if err != nil {
		return nil, err
	}
	return kind, nil
}

func buildOperator(t *testing.T, ctx context.Context) (imageTar string, err error) {
	t.Log("Building the operator")
	// Tag for running this test, for naming resources.
	operatorImage := "etcd-cluster-operator:test"

	// Build the operator.
	out, err := exec.CommandContext(ctx, "docker", "build", "--target=debug", "-t", operatorImage, *fRepoRoot).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w Output: %s", err, out)
	}

	// Bundle the image to a tar.
	tmpDir, err := ioutil.TempDir("", "etcd-cluster-operator-e2e-test")
	if err != nil {
		return "", err
	}

	imageTar = filepath.Join(tmpDir, "etcd-cluster-operator.tar")

	t.Log("Exporting the operator image")
	out, err = exec.CommandContext(ctx, "docker", "save", "-o", imageTar, operatorImage).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w Output: %s", err, out)
	}
	return imageTar, nil
}

func installOperator(t *testing.T, kubectl *kubectlContext, kind *cluster.Context, imageTar string) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.Log("Deleting test namespaces")
		DeleteAllTestNamespaces(t, kubectl)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.Log("Deleting etcd-cluster-operator namespace")
		err := kubectl.Delete("namespace", "eco-system", "--ignore-not-found")
		require.NoError(t, err)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		t.Log("Installing cert-manager")
		err := kubectl.Apply("--validate=false", "--filename=https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml")
		require.NoError(t, err)
		t.Log("Waiting for cert-manager to be ready")
		err = kubectl.Wait("--for=condition=Available", "--timeout=300s", "apiservice", "v1beta1.webhook.cert-manager.io")
		require.NoError(t, err)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		// Ensure CRDs exist in the cluster.
		t.Log("Applying CRDs")
		err := kubectl.Apply("--kustomize", filepath.Join(*fRepoRoot, "config", "crd"))
		require.NoError(t, err)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
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
	}()

	wg.Wait()

	// Deploy the operator.
	t.Log("Applying operator")
	err := kubectl.Apply("--kustomize", filepath.Join(*fRepoRoot, "config", "test"))
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
}

func setupKind(t *testing.T, ctx context.Context, stopped chan struct{}) *kubectlContext {
	ctx, cancel := context.WithCancel(ctx)
	var (
		kind     *cluster.Context
		imageTar string
		wg       sync.WaitGroup
	)
	stoppedKind := make(chan struct{})
	go func() {
		<-stoppedKind
		close(stopped)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		imageTar, err = buildOperator(t, ctx)
		if err != nil {
			assert.NoError(t, err)
			cancel()
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		kind, err = startKind(t, ctx, stoppedKind)
		if err != nil {
			assert.NoError(t, err)
			cancel()
		}
	}()

	wg.Wait()
	require.NoError(t, ctx.Err())
	kubectl := &kubectlContext{
		t:          t,
		configPath: kind.KubeConfigPath(),
	}

	installOperator(t, kubectl, kind, imageTar)

	return kubectl
}

func setupCurrentContext(t *testing.T) *kubectlContext {
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	configPath := filepath.Join(home, ".kube", "config")
	if path, found := os.LookupEnv("KUBECONFIG"); found {
		configPath = path
	}
	return &kubectlContext{
		t:          t,
		configPath: configPath,
	}
}

func TestE2E(t *testing.T) {
	var kubectl *kubectlContext
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		cancel()
	}()
	switch {
	case *fUseKind:
		stoppedKind := make(chan struct{})
		defer func() { <-stoppedKind }()
		defer cancel()
		kubectl = setupKind(t, ctx, stoppedKind)
	case *fUseCurrentContext:
		kubectl = setupCurrentContext(t)
	default:
		t.Skip("Supply either --kind or --current-context to run E2E tests")
	}

	// Delete all existing test namespaces, to free up resources before running new tests.
	DeleteAllTestNamespaces(t, kubectl)

	sampleClusterPath := filepath.Join(*fRepoRoot, "config", "samples", "etcd_v1alpha1_etcdcluster.yaml")

	// Pre-flight check that we can submit etcd API resources, before continuing
	// with the remaining tests.
	// Because the Etcd mutating and validating webhook service may not
	// immediately be responding.
	kubectl = kubectl.WithDefaultNamespace("default")
	var out string
	err := try.Eventually(func() (err error) {
		out, err = kubectl.DryRun(sampleClusterPath)
		return err
	}, time.Second*5, time.Second*1)
	require.NoError(t, err, out)

	// This outer function is needed because call to t.Run does not block if
	// t.Parallel is used in its test function.
	// See https://github.com/golang/go/issues/17791#issuecomment-258527390
	t.Run("Parallel", func(t *testing.T) {
		t.Run("SampleCluster", func(t *testing.T) {
			t.Parallel()
			rl := corev1.ResourceList{
				// 5 node cluster
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("2000Mi"),
			}
			kubectl := kubectl.WithT(t)
			ns, cleanup := NamespaceForTest(t, kubectl, rl)
			defer cleanup()
			sampleClusterTests(t, kubectl.WithDefaultNamespace(ns), sampleClusterPath)
		})
		t.Run("Webhooks", func(t *testing.T) {
			t.Parallel()
			rl := corev1.ResourceList{}
			kubectl := kubectl.WithT(t)
			ns, cleanup := NamespaceForTest(t, kubectl, rl)
			defer cleanup()
			webhookTests(t, kubectl.WithDefaultNamespace(ns))
		})
		t.Run("Persistence", func(t *testing.T) {
			t.Parallel()
			rl := corev1.ResourceList{
				// 1-node cluster
				// set and get jobs
				corev1.ResourceCPU:    resource.MustParse("300m"),
				corev1.ResourceMemory: resource.MustParse("250Mi"),
			}
			kubectl := kubectl.WithT(t)
			ns, cleanup := NamespaceForTest(t, kubectl, rl)
			defer cleanup()
			persistenceTests(t, kubectl.WithDefaultNamespace(ns))
		})
		t.Run("ScaleDown", func(t *testing.T) {
			t.Parallel()
			rl := corev1.ResourceList{
				// 3-node cluster
				// set and get jobs
				corev1.ResourceCPU:    resource.MustParse("700m"),
				corev1.ResourceMemory: resource.MustParse("650Mi"),
			}
			kubectl := kubectl.WithT(t)
			ns, cleanup := NamespaceForTest(t, kubectl, rl)
			defer cleanup()
			scaleDownTests(t, kubectl.WithDefaultNamespace(ns))
		})
		t.Run("Backup", func(t *testing.T) {
			t.Parallel()
			rl := corev1.ResourceList{
				// 1-node cluster
				// set job
				corev1.ResourceCPU:    resource.MustParse("300m"),
				corev1.ResourceMemory: resource.MustParse("250Mi"),
			}
			ns, cleanup := NamespaceForTest(t, kubectl, rl)
			defer cleanup()
			backupTests(t, kubectl.WithT(t).WithDefaultNamespace(ns))
		})
		t.Run("Version", func(t *testing.T) {
			t.Parallel()
			rl := corev1.ResourceList{
				// 3-node cluster
				// set and get jobs
				corev1.ResourceCPU:    resource.MustParse("700m"),
				corev1.ResourceMemory: resource.MustParse("650Mi"),
			}
			kubectl := kubectl.WithT(t)
			ns, cleanup := NamespaceForTest(t, kubectl, rl)
			defer cleanup()
			versionTests(t, kubectl.WithDefaultNamespace(ns))
		})
	})
}

func backupTests(t *testing.T, kubectl *kubectlContext) {
	t.Log("Given a one node cluster.")
	err := kubectl.Apply("--filename", filepath.Join(*fRepoRoot, "config", "test", "e2e", "backup", "etcdcluster.yaml"))
	require.NoError(t, err)

	t.Log("Containing data.")
	out, err := etcdctlInCluster(
		kubectl,
		time.Minute*2,
		"e2e-backup-cluster",
		"put", "--", "foo", "bar",
	)
	require.NoError(t, err, out)

	t.Log("A backup can be taken to a local disk.")
	err = kubectl.Apply("--filename", filepath.Join(*fRepoRoot, "config", "test", "e2e", "backup", "etcdbackup.yaml"))
	require.NoError(t, err)

	kubectlSystem := kubectl.WithDefaultNamespace("eco-system")
	var podNames string
	err = try.Eventually(func() (err error) {
		podNames, err = kubectlSystem.Get("pods", "--output=name")
		return err
	}, time.Minute, time.Second*5)
	require.NoError(t, err)
	podNameList := strings.Split(strings.TrimSpace(podNames), "\n")
	require.Len(t, podNameList, 1)
	podName := strings.TrimPrefix(podNameList[0], "pod/")

	t.Log("And it will be persisted in the expected location.")
	err = try.Eventually(func() (err error) {
		out, err = kubectlSystem.Exec(podName, "ls", "/tmp/backups", "-c", "manager")
		return err
	}, time.Minute*2, time.Second*10)
	require.NoError(t, err)
	t.Log(string(out))
	require.Len(t, strings.Split(string(out), "\n"), 2)
}

func webhookTests(t *testing.T, kubectl *kubectlContext) {
	t.Run("Defaulting", func(t *testing.T) {
		t.Parallel()
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
				kubectl := kubectl.WithT(t)
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
		t.Parallel()
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
				kubectl := kubectl.WithT(t)
				lPath := filepath.Join(*fRepoRoot, "config/test/e2e", tc.path)
				out, err := kubectl.DryRun(lPath)
				assert.Regexp(
					t,
					`^Error from server \(spec.storage.volumeClaimTemplate.storageClassName: Required value\):`,
					err,
				)
				assert.Empty(t, out)
			})
		}
	})

}

func sampleClusterTests(t *testing.T, kubectl *kubectlContext, sampleClusterPath string) {
	err := kubectl.Apply(
		"--filename", filepath.Join(*fRepoRoot, "internal", "test", "e2e", "fixtures", "cluster-client-service.yaml"),
		"--filename", sampleClusterPath,
	)
	require.NoError(t, err)

	t.Run("EtcdClusterAvailability", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
		defer cancel()

		etcdClient, err := etcd.New(etcdConfig)
		require.NoError(t, err)

		err = try.Eventually(func() error {
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
	})

	t.Run("EtcdClusterStatus", func(t *testing.T) {
		kubectl := kubectl.WithT(t)
		err := try.Eventually(func() error {
			t.Log("")
			members, err := kubectl.Get("etcdcluster", "my-cluster", "-o=jsonpath='{.status.members...name}'")
			if err != nil {
				return err
			}
			// Don't assert on exact members, just that we have three of them.
			numMembers := len(strings.Split(members, " "))
			if numMembers != 3 {
				return errors.New(fmt.Sprintf("Expected etcd member list to have three members. Had %d.", numMembers))
			}
			return nil
		}, time.Minute*2, time.Second*10)
		require.NoError(t, err)
	})

	t.Run("ScaleUp", func(t *testing.T) {
		kubectl := kubectl.WithT(t)
		// Attempt to scale up to five nodes
		err = kubectl.Scale("etcdcluster/my-cluster", 5)
		require.NoError(t, err)

		etcdClient, err := etcd.New(etcdConfig)
		require.NoError(t, err)

		err = try.Eventually(func() error {
			t.Log("Listing etcd members")
			members, err := etcd.NewMembersAPI(etcdClient).List(context.Background())
			if err != nil {
				return err
			}

			if len(members) != 5 {
				return errors.New(fmt.Sprintf("expected %d etcd peers, got %d", 5, len(members)))
			}

			for _, member := range members {
				if len(member.ClientURLs) == 0 {
					return errors.New("peer has no client URLs")
				}
				if len(member.PeerURLs) == 0 {
					return errors.New("peer has no peer URLs")
				}
				if member.ID == "" {
					return errors.New("peer has no ID")
				}
				if member.Name == "" {
					return errors.New("peer has no Name")
				}
			}
			return nil
		}, time.Minute*2, time.Second*10)
		require.NoError(t, err)
	})
}

func persistenceTests(t *testing.T, kubectl *kubectlContext) {
	t.Log("Given a 1-node cluster.")
	configPath := filepath.Join(*fRepoRoot, "config", "test", "e2e", "persistence")
	err := kubectl.Apply("--filename", configPath)
	require.NoError(t, err)

	t.Log("Containing data.")
	expectedValue := "foobarbaz"

	out, err := etcdctlInCluster(
		kubectl,
		time.Minute*2,
		"cluster1",
		"put", "--", "foo", expectedValue,
	)
	require.NoError(t, err, out)

	t.Log("If the cluster is deleted.")
	err = kubectl.Delete("etcdcluster", "cluster1", "--wait")
	require.NoError(t, err)

	t.Log("And all the cluster pods terminate.")
	err = try.Eventually(func() error {
		out, err := kubectl.Get(
			"pods",
			"--selector", "etcd.improbable.io/cluster-name=cluster1",
			"--output", "go-template={{ len .items }}",
		)
		if err != nil {
			return err
		}
		if out != "0" {
			return errors.New("expected 0 pods, got: " + out)
		}
		return nil
	}, time.Minute, time.Second*5)
	require.NoError(t, err)

	t.Log("The cluster can be restored.")
	err = kubectl.Apply("--filename", configPath)
	require.NoError(t, err)

	t.Log("And the data is still available.")
	out, err = etcdctlInCluster(
		kubectl,
		time.Minute*2,
		"cluster1",
		"get", "--print-value-only", "--", "foo",
	)
	require.NoError(t, err, out)
	assert.Equal(t, expectedValue+"\n", out)
}

func scaleDownTests(t *testing.T, kubectl *kubectlContext) {
	t.Log("Given a 3-node cluster.")
	configPath := filepath.Join(*fRepoRoot, "config", "samples", "etcd_v1alpha1_etcdcluster.yaml")
	err := kubectl.Apply("--filename", configPath)
	require.NoError(t, err)
	const expectedReplicasOriginal = 3
	t.Log("Where all the nodes are up")
	err = try.Eventually(
		func() error {
			out, err := kubectl.Get("etcdcluster", "my-cluster", "-o=jsonpath={.status.replicas}")
			if err != nil {
				return err
			}
			statusReplicas, err := strconv.Atoi(out)
			if err != nil {
				return err
			}
			if statusReplicas != expectedReplicasOriginal {
				return fmt.Errorf("unexpected status.replicas. Wanted: %d, Got: %d", expectedReplicasOriginal, statusReplicas)
			}
			return nil
		},
		time.Minute*2, time.Second*10,
	)
	require.NoError(t, err)

	t.Log("Which contains data")
	const expectedValue = "foobarbaz"

	out, err := etcdctlInCluster(
		kubectl,
		time.Minute*2,
		"my-cluster",
		"put", "--", "foo", expectedValue,
	)
	require.NoError(t, err, out)

	t.Log("If the cluster is scaled down")
	const expectedReplicasNew = 1
	require.True(t, expectedReplicasNew < expectedReplicasOriginal, "Must use a lower replicas count")
	err = kubectl.Scale("etcdcluster/my-cluster", expectedReplicasNew)
	require.NoError(t, err)

	t.Log("The etcdcluster.status is updated when the cluster has been resized.")
	err = try.Eventually(
		func() error {
			out, err := kubectl.Get("etcdcluster", "my-cluster", "-o=jsonpath={.status.replicas}")
			require.NoError(t, err, out)
			statusReplicas, err := strconv.Atoi(out)
			require.NoError(t, err, out)
			if expectedReplicasNew != statusReplicas {
				return fmt.Errorf("unexpected status.replicas. Wanted: %d, Got: %d", expectedReplicasNew, statusReplicas)
			}
			return err
		},
		time.Minute*5, time.Second*10,
	)
	require.NoError(t, err)

	t.Log("And the data is still available.")
	out, err = etcdctlInCluster(
		kubectl,
		time.Minute*2,
		"my-cluster",
		"get", "--print-value-only", "--", "foo",
	)
	require.NoError(t, err, out)
	assert.Equal(t, expectedValue+"\n", out)

	t.Log("The cluster can then be scaled back up")
	err = kubectl.Scale("etcdcluster/my-cluster", expectedReplicasOriginal)
	require.NoError(t, err)

	t.Log("The cluster eventually reports the expected number of peers")
	err = try.Eventually(
		func() error {
			out, err := kubectl.Get("etcdcluster", "my-cluster", "-o=jsonpath={.status.replicas}")
			if err != nil {
				return err
			}
			statusReplicas, err := strconv.Atoi(out)
			if err != nil {
				return err
			}
			if statusReplicas != expectedReplicasOriginal {
				return fmt.Errorf("unexpected status.replicas. Wanted: %d, Got: %d", expectedReplicasOriginal, statusReplicas)
			}
			return nil
		},
		time.Minute*2, time.Second*10,
	)
	require.NoError(t, err)

	t.Log("And new data can be written to the cluster")
	out, err = etcdctlInCluster(
		kubectl,
		time.Minute*2,
		"my-cluster",
		"put", "--", "foo2", expectedValue+"2",
	)
	require.NoError(t, err, out)

}

func versionTests(t *testing.T, kubectl *kubectlContext) {
	t.Log("Given a 3-node cluster.")
	const originalVersion = "3.2.27"
	const newVersion = "3.2.28"
	const expectedReplicas = 3
	cluster1 := test.ExampleEtcdCluster(*kubectl.defaultNamespace)
	cluster1.Spec.Replicas = pointer.Int32Ptr(expectedReplicas)
	cluster1.Spec.Version = originalVersion
	err := kubectl.ApplyObject(cluster1)
	require.NoError(t, err)

	t.Log("Containing data.")
	expectedValue := "foobarbaz"

	out, err := eventuallyInCluster(
		kubectl,
		"set-etcd-value",
		time.Minute*2,
		"quay.io/coreos/etcd:v3.2.28",
		"etcdctl", "--insecure-discovery", "--discovery-srv=cluster1",
		"set", "--", "foo", expectedValue,
	)
	require.NoError(t, err, out)

	t.Log("The EtcdCluster.Status should contain the Etcd Cluster version")
	expectedVersion := semver.Must(semver.NewVersion("3.2.0"))
	err = try.Eventually(
		func() error {
			out, err := kubectl.Get("etcdcluster", "cluster1", "-o=jsonpath={.status.clusterVersion}")
			if err != nil {
				return err
			}
			actualVersion, err := semver.NewVersion(out)
			if err != nil {
				return fmt.Errorf("Invalid version %q: %s", out, err)
			}
			if !expectedVersion.Equal(*actualVersion) {
				return fmt.Errorf(
					"unexpected Status.ClusterVersion. Wanted: %s, Got: %s",
					expectedVersion, actualVersion,
				)
			}
			return nil
		},
		time.Minute*2, time.Second*10,
	)
	require.NoError(t, err)

	t.Log("The EtcdPeer.Status should contain the Etcd server version")
	expectedServerVersion := semver.Must(semver.NewVersion(originalVersion))
	err = try.Eventually(
		func() error {
			out, err := kubectl.Get("etcdpeer", "cluster1-0", "-o=jsonpath={.status.serverVersion}")
			if err != nil {
				return err
			}
			actualVersion, err := semver.NewVersion(out)
			if err != nil {
				return fmt.Errorf("Invalid version %q: %s", out, err)
			}
			if !expectedServerVersion.Equal(*actualVersion) {
				return fmt.Errorf(
					"unexpected Status.serverVersion. Wanted: %s, Got: %s",
					expectedServerVersion, actualVersion,
				)
			}
			return nil
		},
		time.Minute*2, time.Second*10,
	)
	require.NoError(t, err)

	t.Log("If the version is incremented")
	cluster1.Spec.Version = newVersion
	err = kubectl.ApplyObject(cluster1)
	require.NoError(t, err)

	t.Log("The EtcdPeer.Status should contain the new Etcd server version")
	expectedServerVersions := sets.NewString(newVersion)
	err = try.Eventually(
		func() error {
			out, err := kubectl.Get("etcdpeer", "-o=jsonpath={.items[*].status.serverVersion}")
			if err != nil {
				return err
			}
			versions := strings.Split(strings.TrimSpace(out), " ")
			serverVersions := sets.NewString(versions...)

			if !serverVersions.Equal(expectedServerVersions) {
				return fmt.Errorf("Unexpected versions for peers: %v", serverVersions.List())
			}
			return nil
		},
		time.Minute*2, time.Second*10,
	)
	require.NoError(t, err)

}
