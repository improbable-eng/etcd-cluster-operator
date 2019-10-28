package v1alpha1_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	fuzz "github.com/google/gofuzz"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

func TestEtcdCluster(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		var o *v1alpha1.EtcdCluster
		assert.NotPanics(t, o.Default)
	})

	runDefaulterTests(t, &v1alpha1.EtcdCluster{})
}

func TestEtcdPeer(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		var o *v1alpha1.EtcdPeer
		assert.NotPanics(t, o.Default)
	})

	runDefaulterTests(t, &v1alpha1.EtcdPeer{})
}

func runDefaulterTests(t *testing.T, o webhook.Defaulter) {
	t.Run("Fuzz", func(t *testing.T) {
		f := fuzz.New().MaxDepth(10).NilChance(0.5).NumElements(1, 1).Funcs(
			func(o *metav1.TypeMeta, c fuzz.Continue) {
			},
			func(o *metav1.ObjectMeta, c fuzz.Continue) {
			},
			func(o *v1alpha1.EtcdClusterStatus, c fuzz.Continue) {
			},
			func(o *v1alpha1.EtcdPeerStatus, c fuzz.Continue) {
			},
		)
		for i := 0; i < 100; i++ {
			f.Fuzz(o)
			original := o.DeepCopyObject()
			if assert.NotPanics(t, o.Default) {
				assertOnlyNilValuesChanged(t, original, o)
			}
		}
	})
}

// assertOnlyNilValuesChanged checks that changes have only been made to fields
// which had value nil.
func assertOnlyNilValuesChanged(t *testing.T, original, updated interface{}) {
	diff := cmp.Diff(original, updated)
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "-") && !strings.HasSuffix(line, "nil,") {
			t.Errorf("a non-nil field value was overridden in: %s\n", diff)
		}
	}
}
