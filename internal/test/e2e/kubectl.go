package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// kubectlContext wraps shell calls to `kubectl', appropriately handling their results. It is only intended for use
// in tests.
type kubectlContext struct {
	t          *testing.T
	configPath string
	homeDir    string
}

func (k *kubectlContext) do(args ...string) ([]byte, error) {
	if k.homeDir == "" {
		hd, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		k.homeDir = hd
	}

	k.t.Log("Running kubectl " + strings.Join(args, " "))
	cmd := exec.Command("kubectl", args...)
	cmd.Env = append(cmd.Env, "KUBECONFIG="+k.configPath)
	cmd.Env = append(cmd.Env, "HOME="+k.homeDir)
	return cmd.CombinedOutput()
}

// Apply wraps `kubectl apply', returning any error that occurred.
func (k *kubectlContext) Apply(args ...string) error {
	out, err := k.do(append([]string{"apply"}, args...)...)
	k.t.Log(string(out))
	return err
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
