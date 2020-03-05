package e2e

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	etcd "go.etcd.io/etcd/clientv3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"

	semver "github.com/coreos/go-semver/semver"
	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test/try"
)

const (
	expectedClusterSize = 3
)

var (
	fe2eEnabled      = flag.Bool("e2e-enabled", false, "Run these end-to-end tests. By default they are skipped.")
	fRepoRoot        = flag.String("repo-root", "", "The absolute path to the root of the etcd-cluster-operator git repository.")
	fOutputDirectory = flag.String("output-directory", "/tmp/etcd-e2e", "The absolute path to a directory where E2E results logs will be saved.")
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
	if !*fe2eEnabled {
		t.Skip("Supply --e2e-enabled to run E2E tests")
	}
	var kubectl *kubectlContext
	kubectl = setupCurrentContext(t)

	// Delete all existing test namespaces, to free up resources before running new tests.
	require.NoError(t, DeleteAllTestNamespaces(kubectl), "failed to delete test namespaces")

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
				// 4 node cluster
				// set job
				corev1.ResourceCPU:    resource.MustParse("900m"),
				corev1.ResourceMemory: resource.MustParse("850Mi"),
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
		t.Run("Restore", func(t *testing.T) {
			t.Parallel()
			rl := corev1.ResourceList{
				// 3-node cluster
				// set and get jobs
				corev1.ResourceCPU:    resource.MustParse("700m"),
				corev1.ResourceMemory: resource.MustParse("650Mi"),
			}
			ns, cleanup := NamespaceForTest(t, kubectl, rl)
			defer cleanup()
			restoreTests(t, kubectl.WithT(t).WithDefaultNamespace(ns))
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

func restoreTests(t *testing.T, kubectl *kubectlContext) {
	t.Log("Create EtcdRestore from backup in MinIO")
	err := kubectl.Apply("--filename",
		filepath.Join(*fRepoRoot, "config", "test", "e2e", "restore", "etcdrestore.yaml"))
	require.NoError(t, err)

	t.Log("And the data is still available.")
	out, err := etcdctlInCluster(
		kubectl,
		time.Minute*2,
		"restored-cluster",
		"get", "--print-value-only", "--", "foo",
	)
	require.NoError(t, err, out)
	assert.Equal(t, "bar\n", out)
	require.NoError(t, err, out)
}

func backupTests(t *testing.T, kubectl *kubectlContext) {
	t.Log("Given a one node cluster.")
	cluster := test.ExampleEtcdCluster(*kubectl.defaultNamespace)
	cluster.Spec.Replicas = pointer.Int32Ptr(1)
	err := kubectl.ApplyObject(cluster)
	require.NoError(t, err)

	t.Log("Containing data.")
	out, err := etcdctlInCluster(
		kubectl,
		time.Minute*2,
		cluster.Name,
		"put", "--", "foo", "bar",
	)
	require.NoError(t, err, out)

	t.Log("A proxy backup can be taken.")
	backupFileName := fmt.Sprintf("backup-%s.db", randomString(8))
	backup := test.ExampleEtcdBackup(cluster.Namespace)
	backup.Spec.Destination.ObjectURLTemplate = fmt.Sprintf("s3://backups.test.improbable.io/%s?endpoint=http://minio.minio.svc:9000&disableSSL=true&s3ForcePathStyle=true&region=eu-west-2", backupFileName)
	err = kubectl.ApplyObject(backup)
	require.NoError(t, err)

	t.Log("The EtcdBackup runs to completion")
	var actualBackup etcdv1alpha1.EtcdBackup
	err = try.Eventually(func() error {
		out, err := kubectl.Get("etcdbackup", backup.Name, "--output", "json")
		if err != nil {
			return fmt.Errorf("error %q with output %q", err, out)
		}
		if err := json.Unmarshal([]byte(out), &actualBackup); err != nil {
			return fmt.Errorf("error %q unmarshalling %q", err, out)
		}
		switch phase := actualBackup.Status.Phase; phase {
		case etcdv1alpha1.EtcdBackupPhaseCompleted, etcdv1alpha1.EtcdBackupPhaseFailed:
			return nil
		default:
			return fmt.Errorf("unexpected backup phase: %s", phase)
		}
	}, time.Second*10, time.Second*2)
	require.NoError(t, err, "Backup not finished")
	require.Equal(t, actualBackup.Status.Phase, etcdv1alpha1.EtcdBackupPhaseCompleted, "Backup failed")

	t.Log("And the snapshot file has been uploaded")
	minioBackupPath := fmt.Sprintf("/data/backups.test.improbable.io/%s", backupFileName)
	kubectlMinio := kubectl.WithDefaultNamespace("minio")
	err = try.Eventually(func() (err error) {
		out, err = kubectlMinio.Exec(
			"--container", "minio", "minio-0", "--", "ls", minioBackupPath,
		)
		if err != nil {
			return fmt.Errorf("error %q from kubectl exec with output %q", err, out)
		}
		return nil
	}, time.Minute*5, time.Second*10)
	require.NoError(t, err)
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
					`Error from server \(spec.storage.volumeClaimTemplate.storageClassName: Required value\):`,
					err,
				)
				assert.Empty(t, out)
			})
		}
	})

}

func sampleClusterTests(t *testing.T, kubectl *kubectlContext, sampleClusterPath string) {
	err := kubectl.Apply("--filename", sampleClusterPath)
	require.NoError(t, err)

	t.Run("EtcdClusterAvailability", func(t *testing.T) {
		out, err := etcdctlInCluster(
			kubectl,
			time.Minute*2,
			"my-cluster",
			"put", "--", "foo", "sample-cluster-tests-value",
		)
		require.NoError(t, err, out)
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
		// Attempt to scale up to four nodes

		const expectedReplicas = 4
		err := kubectl.Scale("etcdcluster/my-cluster", expectedReplicas)
		require.NoError(t, err)

		t.Log("The etcdcluster.status is updated when the cluster has been resized.")
		err = try.Eventually(
			func() error {
				out, err := kubectl.Get("etcdcluster", "my-cluster", "-o=jsonpath={.status.replicas}")
				require.NoError(t, err, out)
				statusReplicas, err := strconv.Atoi(out)
				require.NoError(t, err, out)
				if expectedReplicas != statusReplicas {
					return fmt.Errorf("unexpected status.replicas. Wanted: %d, Got: %d", expectedReplicas, statusReplicas)
				}
				return err
			},
			time.Minute*5, time.Second*10,
		)
		require.NoError(t, err)

		err = try.Eventually(func() error {
			out, err := etcdctlInCluster(
				kubectl,
				time.Minute*2,
				"my-cluster",
				"member", "list", "--write-out=json",
			)
			if err != nil {
				return fmt.Errorf("etcdctlInCluster error: %s, out: %s", err, out)
			}

			var response etcd.MemberListResponse

			err = json.Unmarshal([]byte(out), &response)
			if err != nil {
				return fmt.Errorf("json.Unmarshal error: %s", err)
			}

			members := response.Members
			if len(members) != expectedReplicas {
				return errors.New(fmt.Sprintf("expected %d etcd peers, got %d", expectedReplicas, len(members)))
			}

			for _, member := range members {
				if len(member.ClientURLs) == 0 {
					return errors.New("peer has no client URLs")
				}
				if len(member.PeerURLs) == 0 {
					return errors.New("peer has no peer URLs")
				}
				if member.ID == 0 {
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

	out, err := etcdctlInCluster(
		kubectl,
		time.Minute*2,
		"cluster1",
		"put", "--", "foo", expectedValue,
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
