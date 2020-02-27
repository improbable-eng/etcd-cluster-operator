package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	testNameLabelKey = "e2e.etcd.improbable.io/test-name"
	// The etcd image containing etcdctl
	etcdctlImage = "quay.io/coreos/etcd:v3.3.18"
	// charSet defines the alphanumeric set for random string generation.
	// These must encode to a single byte in UTF-8 for compatibility with the
	// randomString() function.
	charSet = "0123456789abcdefghijklmnopqrstuvwxyz"
)

var (
	rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// randomString returns a random alphanumeric string.
// Copied from https://github.com/kubernetes-sigs/cluster-api/blob/v0.2.9/util/util.go#L68
func randomString(n int) string {
	result := make([]byte, n)
	for i := range result {
		result[i] = charSet[rnd.Intn(len(charSet))]
	}
	return string(result)
}

// kubectlContext wraps shell calls to `kubectl', appropriately handling their results. It is only intended for use
// in tests.
type kubectlContext struct {
	t                *testing.T
	configPath       string
	homeDir          string
	defaultNamespace *string
}

func (k *kubectlContext) do(args ...string) ([]byte, error) {
	if k.homeDir == "" {
		hd, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		k.homeDir = hd
	}

	if k.defaultNamespace != nil {
		args = append([]string{"--namespace", *k.defaultNamespace}, args...)
	}
	commandLine := "kubectl " + strings.Join(args, " ")
	k.t.Log("Running ", commandLine)
	cmd := exec.Command("kubectl", args...)
	cmd.Env = append(cmd.Env, "KUBECONFIG="+k.configPath)
	cmd.Env = append(cmd.Env, "HOME="+k.homeDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		err = fmt.Errorf("error %q from command %q with stderr %q", err, commandLine, stderr.String())
	}
	if stderr.Len() > 0 {
		k.t.Logf("STDERR: %s", stderr.String())
	}
	return out, err
}

// Apply wraps `kubectl apply', returning any error that occurred.
func (k *kubectlContext) Apply(args ...string) error {
	out, err := k.do(append([]string{"apply"}, args...)...)
	k.t.Log(string(out))
	return err
}

// ApplyObject serializes an object to file and applies it `kubectl apply',
// returning any error that occurred.
func (k *kubectlContext) ApplyObject(o metav1.Object) error {
	f, err := ioutil.TempFile("", "etcd-e2e."+o.GetNamespace()+"."+o.GetName()+".json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	encoder := json.NewEncoder(f)
	err = encoder.Encode(o)
	if err != nil {
		return err
	}
	return k.Apply("--filename", f.Name())
}

// Patch wraps `kubectl patch', returning any error that occurred.
func (k *kubectlContext) Patch(args ...string) error {
	out, err := k.do(append([]string{"patch"}, args...)...)
	k.t.Log(string(out))
	return err
}

// Get wraps `kubectl get', returning the unparsed output & any error that occurred.
func (k *kubectlContext) Get(args ...string) (string, error) {
	out, err := k.do(append([]string{"get"}, args...)...)
	return string(out), err
}

// Wait wraps `kubectl wait', returning any error that occurred.
func (k *kubectlContext) Wait(args ...string) error {
	out, err := k.do(append([]string{"wait"}, args...)...)
	k.t.Log(string(out))
	return err
}

// Run wraps `kubectl run', returning the unparsed output & any error that occurred.
func (k *kubectlContext) Run(args ...string) (string, error) {
	out, err := k.do(append([]string{"run"}, args...)...)
	return string(out), err
}

// Delete wraps `kubectl delete', returning any error that occurred.
func (k *kubectlContext) Delete(args ...string) error {
	out, err := k.do(append([]string{"delete"}, args...)...)
	k.t.Log(string(out))
	return err
}

// Exec wraps `kubectl exec', returning the output of the executed command
func (k *kubectlContext) Exec(podName string, cmd ...string) (string, error) {
	out, err := k.do(append([]string{"exec", podName}, cmd...)...)
	return string(out), err
}

// DryRun wraps `kubectl apply --server-dry-run', returning the unparsed output & any error that occurred.
func (k *kubectlContext) DryRun(filename string) (string, error) {
	out, err := k.do("apply", "--server-dry-run", "--output", "yaml", "--filename", filename)
	return string(out), err
}

// Scale wraps 'kubectl scale', returning any errors that occurred
func (k *kubectlContext) Scale(resource string, scale uint) error {
	out, err := k.do("scale", fmt.Sprintf("--replicas=%d", scale), resource)
	k.t.Log(string(out))
	return err
}

// Create wraps `kubectl create', returning the unparsed output & any errors that occurred.
func (k *kubectlContext) Create(args ...string) (string, error) {
	out, err := k.do(append([]string{"create"}, args...)...)
	return string(out), err
}

// Logs wraps `kubectl logs', returning the unparsed output & any errors that occurred.
func (k *kubectlContext) Logs(args ...string) (string, error) {
	out, err := k.do(append([]string{"logs"}, args...)...)
	return string(out), err
}

// Label wraps `kubectl label', returning the unparsed output & any errors that occurred.
func (k *kubectlContext) Label(args ...string) (string, error) {
	out, err := k.do(append([]string{"label"}, args...)...)
	return string(out), err
}

// WithT returns a copy of k with a new testing context.
// This ensures that messages logged with t.Log in this module are associated with the correct sub-test.
// You should use this in any sub-test.
func (k *kubectlContext) WithT(t *testing.T) *kubectlContext {
	return &kubectlContext{
		t:                t,
		configPath:       k.configPath,
		homeDir:          k.homeDir,
		defaultNamespace: k.defaultNamespace,
	}
}

// WithDefaultNamespace returns a kubectl where `--namespace ns` will be
// prepended to all the other supplied arguments.
func (k *kubectlContext) WithDefaultNamespace(ns string) *kubectlContext {
	return &kubectlContext{
		t:                k.t,
		configPath:       k.configPath,
		homeDir:          k.homeDir,
		defaultNamespace: &ns,
	}
}

// ClusterInfoDump saves calls `kubectl cluster-info dump` to save all the
// pod-logs from the supplied namespaces.
// TODO(wallrj) W use --all-namespaces here because the --namespaces flag doesn't work.
// See https://github.com/kubernetes/kubernetes/issues/72088
func (k *kubectlContext) ClusterInfoDump(outputDirectory string, namespaces ...string) error {
	_, err := k.do("cluster-info", "dump", "--all-namespaces", "--output-directory", outputDirectory)
	if err != nil {
		return err
	}
	entries, err := ioutil.ReadDir(outputDirectory)
	if err != nil {
		return err
	}
	entriesFound := sets.NewString()
	for _, entry := range entries {
		entriesFound.Insert(entry.Name())
	}
	entriesToKeep := sets.NewString(namespaces...)
	entriesToKeep.Insert("eco-system", "nodes.json")
	entriesToDelete := entriesFound.Difference(entriesToKeep)
	for _, entry := range entriesToDelete.List() {
		path := path.Join(outputDirectory, entry)
		err := os.RemoveAll(path)
		if err != nil {
			return err
		}
	}
	return nil
}

// NamespaceForTest creates a new namespace with a name derived from the current
// running test.
// It adds a label, so that all such namespaces can easily be found.
// And if a namespace with that label already exists, it deletes it first to
// cleanup any resources left over from the previous test.
func NamespaceForTest(t *testing.T, kubectl *kubectlContext, rl corev1.ResourceList) (string, func()) {
	testName := t.Name()
	name := testName
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ToLower(name)
	label := fmt.Sprintf("%s=%s", testNameLabelKey, name)
	rq := &corev1.ResourceQuota{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ResourceQuota",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compute-resources",
			Namespace: name,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: rl.DeepCopy(),
		},
	}
	cleanup := func() {
		kubectl := kubectl.WithDefaultNamespace(name)
		t.Logf("Saving logs for namespace: %s", name)
		kubectl.ClusterInfoDump(path.Join(*fOutputDirectory, name), name)
		t.Logf("Evicting all pods from namespace: %s", name)
		rq.Spec.Hard[corev1.ResourcePods] = resource.MustParse("0")
		err := kubectl.ApplyObject(rq)
		require.NoError(t, err)
		err = kubectl.Delete("pods", "--all", "--wait=false")
		require.NoError(t, err)
	}
	t.Log("Creating new namespace for test")
	out, err := kubectl.Create("namespace", name)
	require.NoError(t, err, out)

	t.Log("Labelling namespace")
	out, err = kubectl.Label("namespace", name, label)
	require.NoError(t, err, out)

	t.Log("Adding resource quota")
	kubectl = kubectl.WithDefaultNamespace(name)
	err = kubectl.ApplyObject(rq)
	require.NoError(t, err)

	return name, cleanup
}

func DeleteAllTestNamespaces(kubectl *kubectlContext) error {
	return kubectl.Delete("namespace", "--selector", testNameLabelKey, "--wait")
}

// eventuallyInCluster runs a command as a Job in the current Kubernetes cluster namespace.
func eventuallyInCluster(kubectl *kubectlContext, name string, deadline time.Duration, image string, command ...string) (_ string, reterr error) {
	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "container1",
							Image:   image,
							Command: command,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"cpu":    resource.MustParse("100m"),
									"memory": resource.MustParse("50Mi"),
								},
								Limits: corev1.ResourceList{
									"cpu":    resource.MustParse("100m"),
									"memory": resource.MustParse("50Mi"),
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
			},
		},
	}

	f, err := ioutil.TempFile("", "e2e-job."+name+".json")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()

	encoder := json.NewEncoder(f)
	err = encoder.Encode(job)
	if err != nil {
		return "", err
	}
	err = kubectl.Apply("--filename", f.Name())
	if err != nil {
		return "", err
	}
	defer func() {
		err := kubectl.Delete("--filename", f.Name())
		if err != nil {
			reterr = kerrors.NewAggregate([]error{
				reterr,
				fmt.Errorf("error while deleting job in eventuallyInCluster: %s ", err),
			})
		}
	}()
	err = kubectl.Wait("job", name, "--for", fmt.Sprintf("condition=%s", batchv1.JobComplete), "--timeout", deadline.String())
	if err != nil {
		return "", err
	}
	return kubectl.Logs("--selector", fmt.Sprintf("job-name=%s", name))
}

// etcdctlInCluster executes etcdctl as a Job in the cluster and waits for its output
func etcdctlInCluster(kubectl *kubectlContext, deadline time.Duration, clusterName string, command ...string) (string, error) {
	return eventuallyInCluster(
		kubectl,
		"etcdctl-"+randomString(8),
		deadline,
		etcdctlImage,
		append([]string{"/usr/bin/env", "ETCDCTL_API=3", "etcdctl", "--insecure-discovery", "--discovery-srv", clusterName}, command...)...,
	)
}
