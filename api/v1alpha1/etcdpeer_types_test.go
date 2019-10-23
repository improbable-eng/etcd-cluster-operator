package v1alpha1

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func exampleEtcdPeer(namespace string) *EtcdPeer {
	return &EtcdPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bees",
			Namespace: namespace,
		},
		Spec: EtcdPeerSpec{
			ClusterName: "my-cluster",
			Bootstrap: &Bootstrap{
				Static: &StaticBootstrap{
					InitialCluster: []InitialClusterMember{
						{
							Name: "bees",
							Host: fmt.Sprintf("bees.my-cluster.%s.svc", namespace),
						},
						{
							Name: "magic",
							Host: fmt.Sprintf("magic.my-cluster.%s.svc", namespace),
						},
						{
							Name: "goose",
							Host: fmt.Sprintf("goose.my-cluster.%s.svc", namespace),
						},
					},
				},
			},
			Storage: &EtcdPeerStorage{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: pointer.StringPtr("example-class"),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": resource.MustParse("999Gi"),
						},
					},
				},
			},
		},
	}
}

func TestValidateCreate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		o := exampleEtcdPeer("ns1")
		err := o.ValidateCreate()
		if !assert.NoError(t, err) {
			t.Log(err)
		}
	})
	t.Run("spec.storage missing", func(t *testing.T) {
		o := exampleEtcdPeer("ns1")
		o.Spec.Storage = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("spec.storage.volumeClaimTemplate missing", func(t *testing.T) {
		o := exampleEtcdPeer("ns1")
		o.Spec.Storage.VolumeClaimTemplate = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("spec.storage.volumeClaimTemplate.storageClassName missing", func(t *testing.T) {
		o := exampleEtcdPeer("ns1")
		o.Spec.Storage.VolumeClaimTemplate.StorageClassName = nil
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
	t.Run("spec.storage.volumeClaimTemplate.resources.storage missing", func(t *testing.T) {
		o := exampleEtcdPeer("ns1")
		delete(o.Spec.Storage.VolumeClaimTemplate.Resources.Requests, "storage")
		err := o.ValidateCreate()
		if assert.Error(t, err) {
			t.Log(err)
		}
	})
}
