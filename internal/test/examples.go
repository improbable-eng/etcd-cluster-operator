package test

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

// ExampleEtcdPeer returns a valid example for testing purposes
func ExampleEtcdPeer(namespace string) *v1alpha1.EtcdPeer {
	return &v1alpha1.EtcdPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bees",
			Namespace: namespace,
		},
		Spec: v1alpha1.EtcdPeerSpec{
			ClusterName: "my-cluster",
			Bootstrap: &v1alpha1.Bootstrap{
				Static: &v1alpha1.StaticBootstrap{
					InitialCluster: []v1alpha1.InitialClusterMember{
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
			Storage: &v1alpha1.EtcdPeerStorage{
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
