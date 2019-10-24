package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCreate(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		o := ExampleEtcdPeer("ns1")
		err := o.ValidateCreate()
		if !assert.NoError(t, err) {
			t.Log(err)
		}
	})
	t.Run("StorageMissing", func(t *testing.T) {
		o := ExampleEtcdPeer("ns1")
		o.Spec.Storage = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("VolumeClaimTemplateMissing", func(t *testing.T) {
		o := ExampleEtcdPeer("ns1")
		o.Spec.Storage.VolumeClaimTemplate = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("StorageClassNameMissing", func(t *testing.T) {
		o := ExampleEtcdPeer("ns1")
		o.Spec.Storage.VolumeClaimTemplate.StorageClassName = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("ResourcesStorageMissing", func(t *testing.T) {
		o := ExampleEtcdPeer("ns1")
		delete(o.Spec.Storage.VolumeClaimTemplate.Resources.Requests, "storage")
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
}
