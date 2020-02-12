package v1alpha1_test

import (
	"testing"

	"github.com/coreos/go-semver/semver"
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
	t.Run("VersionInvalid", func(t *testing.T) {
		o := test.ExampleEtcdCluster("ns1")
		o.Spec.Version = "foo"
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("VersionUnsupported", func(t *testing.T) {
		o := test.ExampleEtcdCluster("ns1")
		o.Spec.Version = "4.0.0"
		err := o.ValidateCreate()
		if assert.Error(t, err) {
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
	t.Run("ReservedPodAnnotationUsed", func(t *testing.T) {
		o := test.ExampleEtcdCluster("ns1")

		o.Spec.PodTemplate = &v1alpha1.EtcdPodTemplateSpec{
			Metadata: &v1alpha1.EtcdPodTemplateObjectMeta{
				Annotations: map[string]string{
					"etcd.improbable.io/test": "some-value",
				},
			},
		}

		err := o.ValidateCreate()
		if !assert.Error(t, err) {
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
			name: "VersionPatchIncrement",
			modifier: func(o *v1alpha1.EtcdCluster) {
				v := semver.Must(semver.NewVersion(o.Spec.Version))
				v.Patch += 1
				o.Spec.Version = v.String()
			},
		},
		{
			name: "VersionPatchDecrement",
			modifier: func(o *v1alpha1.EtcdCluster) {
				v := semver.Must(semver.NewVersion(o.Spec.Version))
				v.Patch -= 1
				o.Spec.Version = v.String()
			},
		},
		{
			name: "VersionMinorIncrement",
			modifier: func(o *v1alpha1.EtcdCluster) {
				v := semver.Must(semver.NewVersion(o.Spec.Version))
				v.Minor += 1
				o.Spec.Version = v.String()
			},
		},
		{
			name: "VersionMinorDecrement",
			modifier: func(o *v1alpha1.EtcdCluster) {
				v := semver.Must(semver.NewVersion(o.Spec.Version))
				v.Minor += 1
				o.Spec.Version = v.String()
			},
		},
		{
			name: "VersionMajorIncrement",
			modifier: func(o *v1alpha1.EtcdCluster) {
				v := semver.Must(semver.NewVersion(o.Spec.Version))
				v.Major += 1
				o.Spec.Version = v.String()
			},
			err: "^Unsupported changes:",
		},
		{
			name: "VersionMajorDecrement",
			modifier: func(o *v1alpha1.EtcdCluster) {
				v := semver.Must(semver.NewVersion(o.Spec.Version))
				v.Major -= 1
				o.Spec.Version = v.String()
			},
			err: "^Unsupported changes:",
		},
		{
			// TODO Support pod annotation modification https://github.com/improbable-eng/etcd-cluster-operator/issues/109
			name: "ModifyPodSpecAnnotation",
			modifier: func(o *v1alpha1.EtcdCluster) {
				o.Spec.PodTemplate = &v1alpha1.EtcdPodTemplateSpec{
					Metadata: &v1alpha1.EtcdPodTemplateObjectMeta{
						Annotations: map[string]string{
							"new-annotation": "some-value",
						},
					},
				}
			},
			err: "^Unsupported changes:",
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
				storage, found := o.Spec.Storage.VolumeClaimTemplate.Resources.Requests["storage"]
				if !found {
					panic("A storage request must be set for this test")
				}
				storage.Add(resource.MustParse("1Mi"))
				o.Spec.Storage.VolumeClaimTemplate.Resources.Requests["storage"] = storage
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
	t.Run("VersionInvalid", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Version = "foo"
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("VersionUnsupported", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Version = "4.0.0"
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("StorageMissing", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Storage = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("VolumeClaimTemplateMissing", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Storage.VolumeClaimTemplate = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("StorageClassNameMissing", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		o.Spec.Storage.VolumeClaimTemplate.StorageClassName = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("ResourcesStorageMissing", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")
		delete(o.Spec.Storage.VolumeClaimTemplate.Resources.Requests, "storage")
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("ReservedPodAnnotationUsed", func(t *testing.T) {
		o := test.ExampleEtcdPeer("ns1")

		o.Spec.PodTemplate = &v1alpha1.EtcdPodTemplateSpec{
			Metadata: &v1alpha1.EtcdPodTemplateObjectMeta{
				Annotations: map[string]string{
					"etcd.improbable.io/test": "some-value",
				},
			},
		}

		err := o.ValidateCreate()
		if !assert.Error(t, err) {
			t.Log(err)
		}
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
			name: "UnsupportedChange/Version",
			modifier: func(o *v1alpha1.EtcdPeer) {
				v := semver.Must(semver.NewVersion(o.Spec.Version))
				v.Patch += 1
				o.Spec.Version = v.String()
			},
			err: `^Unsupported changes:`,
		},
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
				storage, found := o.Spec.Storage.VolumeClaimTemplate.Resources.Requests["storage"]
				if !found {
					panic("A storage request must be set for this test")
				}
				storage.Add(resource.MustParse("1Mi"))
				o.Spec.Storage.VolumeClaimTemplate.Resources.Requests["storage"] = storage
			},
			err: `^Unsupported changes:`,
		},
		{
			// TODO Support pod annotation modification https://github.com/improbable-eng/etcd-cluster-operator/issues/109
			name: "ModifyPodSpecAnnotation",
			modifier: func(o *v1alpha1.EtcdPeer) {
				o.Spec.PodTemplate = &v1alpha1.EtcdPodTemplateSpec{
					Metadata: &v1alpha1.EtcdPodTemplateObjectMeta{
						Annotations: map[string]string{
							"new-annotation": "some-value",
						},
					},
				}
			},
			err: "^Unsupported changes:",
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
