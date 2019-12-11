package v1alpha1_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/test"
)

func TestEtcdCluster_ValidateCreate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		o := test.ExampleEtcdCluster("ns1")
		err := o.ValidateCreate()
		if !assert.NoError(t, err) {
			t.Log(err)
		}
	})
	t.Run("StorageMissing", func(t *testing.T) {
		o := test.ExampleEtcdCluster("ns1")
		o.Spec.Storage = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("VolumeClaimTemplateMissing", func(t *testing.T) {
		o := test.ExampleEtcdCluster("ns1")
		o.Spec.Storage.VolumeClaimTemplate = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("StorageClassNameMissing", func(t *testing.T) {
		o := test.ExampleEtcdCluster("ns1")
		o.Spec.Storage.VolumeClaimTemplate.StorageClassName = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("ResourcesStorageMissing", func(t *testing.T) {
		o := test.ExampleEtcdCluster("ns1")
		delete(o.Spec.Storage.VolumeClaimTemplate.Resources.Requests, "storage")
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
}

// TODO Incomplete.
// Need to test changes for all PVC fields.
// Ideally without exhaustively testing them all here.
func TestEtcdCluster_ValidateUpdate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		modifier func(*v1alpha1.EtcdCluster)
		err      string
	}{
		{
			name: "ScaleUp",
			modifier: func(o *v1alpha1.EtcdCluster) {
				*o.Spec.Replicas += 1
			},
		},
		{
			name: "ScaleDown",
			modifier: func(o *v1alpha1.EtcdCluster) {
				*o.Spec.Replicas -= 1
			},
		},
		{
			name: "UnsupportedChange/StorageClassName",
			modifier: func(o *v1alpha1.EtcdCluster) {
				*o.Spec.Storage.VolumeClaimTemplate.StorageClassName += "-changed"
			},
			err: `^Unsupported changes:`,
		},
		{
			name: "UnsupportedChange/ResourcesStorage",
			modifier: func(o *v1alpha1.EtcdCluster) {
				o.Spec.Storage.VolumeClaimTemplate.Resources.Requests["storage"] = resource.MustParse("1Mi")
			},
			err: `^Unsupported changes:`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o1 := test.ExampleEtcdCluster("ns1")
			o2 := o1.DeepCopy()
			tc.modifier(o2)
			err := o2.ValidateUpdate(o1)
			if err != nil {
				t.Log(err)
			}
			if tc.err != "" {
				assert.Regexp(t, tc.err, err, "unexpected error message")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
		})
	}

}

func TestEtcdPeer_ValidateCreate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		err := o.ValidateCreate()
		if !assert.NoError(t, err) {
			t.Log(err)
		}
	})
	t.Run("Error/StorageMissing", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Storage = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("Error/VolumeClaimTemplateMissing", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Storage.VolumeClaimTemplate = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("Error/StorageClassNameMissing", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Storage.VolumeClaimTemplate.StorageClassName = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("Error/ResourcesStorageMissing", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		delete(o.Spec.Storage.VolumeClaimTemplate.Resources.Requests, "storage")
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("Error/BootstrapInitialClusterStateNewMissingStatic", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Bootstrap = &v1alpha1.Bootstrap{
			InitialClusterState: v1alpha1.InitialClusterStateNew.Pointer(),
		}
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("Error/BootstrapInitialClusterStateExistingWithStatic", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Bootstrap = &v1alpha1.Bootstrap{
			InitialClusterState: v1alpha1.InitialClusterStateExisting.Pointer(),
			Static: &v1alpha1.StaticBootstrap{
				InitialCluster: []v1alpha1.InitialClusterMember{
					{Name: "foo", Host: "bar"},
				},
			},
		}
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("Success/BootstrapInitialClusterStateNewWithStatic", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Bootstrap = &v1alpha1.Bootstrap{
			InitialClusterState: v1alpha1.InitialClusterStateNew.Pointer(),
			Static: &v1alpha1.StaticBootstrap{
				InitialCluster: []v1alpha1.InitialClusterMember{
					{Name: "foo", Host: "bar"},
				},
			},
		}
		err := o.ValidateCreate()
		assert.NoError(t, err)
	})
}

// TODO Incomplete.
// Need to test changes for all PVC fields.
// Ideally without exhaustively testing them all here.
func TestEtcdPeer_ValidateUpdate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		modifier func(*v1alpha1.EtcdPeer)
		err      string
	}{
		{
			name: "UnsupportedChange/StorageClassName",
			modifier: func(o *v1alpha1.EtcdPeer) {
				*o.Spec.Storage.VolumeClaimTemplate.StorageClassName += "-changed"
			},
			err: `^Unsupported changes:`,
		},
		{
			name: "UnsupportedChange/ResourcesStorage",
			modifier: func(o *v1alpha1.EtcdPeer) {
				o.Spec.Storage.VolumeClaimTemplate.Resources.Requests["storage"] = resource.MustParse("1Mi")
			},
			err: `^Unsupported changes:`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o1 := test.ExampleEtcdPeer("ns1")
			o2 := o1.DeepCopy()
			tc.modifier(o2)
			err := o2.ValidateUpdate(o1)
			if err != nil {
				t.Log(err)
			}
			if tc.err != "" {
				assert.Regexp(t, tc.err, err, "unexpected error message")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
		})
	}

}
