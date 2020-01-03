package test

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

// ExampleEtcdCluster returns a valid example for testing purposes.
func ExampleEtcdCluster(namespace string) *etcdv1alpha1.EtcdCluster {
	return &etcdv1alpha1.EtcdCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: namespace,
		},
		Spec: etcdv1alpha1.EtcdClusterSpec{
			Replicas: pointer.Int32Ptr(3),
			Storage: &etcdv1alpha1.EtcdPeerStorage{
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

// ExampleEtcdPeer returns a valid example for testing purposes.
func ExampleEtcdPeer(namespace string) *etcdv1alpha1.EtcdPeer {
	return &etcdv1alpha1.EtcdPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bees",
			Namespace: namespace,
		},
		Spec: etcdv1alpha1.EtcdPeerSpec{
			ClusterName: "my-cluster",
			Bootstrap: &etcdv1alpha1.Bootstrap{
				Static: &etcdv1alpha1.StaticBootstrap{
					InitialCluster: []etcdv1alpha1.InitialClusterMember{
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
				InitialClusterState: etcdv1alpha1.InitialClusterStateNew,
			},
			Storage: &etcdv1alpha1.EtcdPeerStorage{
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

func ExampleEtcdBackupSchedule(namespace string) *etcdv1alpha1.EtcdBackupSchedule {
	return &etcdv1alpha1.EtcdBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup-foo",
			Namespace: namespace,
		},
		Spec: etcdv1alpha1.EtcdBackupScheduleSpec{
			Schedule: "* * * * *",
			BackupTemplate: etcdv1alpha1.EtcdBackupSpec{
				ClusterEndpoints: []etcdv1alpha1.EtcdClusterEndpoint{{
					Port:   2379,
					Host:   "my-cluster.com",
					Scheme: "http",
				}},
			},
		},
	}
}
