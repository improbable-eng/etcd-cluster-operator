package test

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

// ExampleEtcdCluster returns a valid example for testing purposes.
func ExampleEtcdCluster(namespace string) *etcdv1alpha1.EtcdCluster {
	return &etcdv1alpha1.EtcdCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "EtcdCluster",
			APIVersion: "etcd.improbable.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster1",
			Namespace: namespace,
		},
		Spec: etcdv1alpha1.EtcdClusterSpec{
			Replicas: pointer.Int32Ptr(3),
			Version:  "3.4.999",
			Storage: &etcdv1alpha1.EtcdPeerStorage{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: pointer.StringPtr("standard"),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": resource.MustParse("1Mi"),
						},
					},
				},
			},
			PodTemplate: &etcdv1alpha1.EtcdPodTemplateSpec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"cpu":    resource.MustParse("200m"),
						"memory": resource.MustParse("200Mi"),
					},
					Limits: corev1.ResourceList{
						"cpu":    resource.MustParse("200m"),
						"memory": resource.MustParse("200Mi"),
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
			Version:     "3.4.999",
			Bootstrap: &etcdv1alpha1.Bootstrap{
				Static: &etcdv1alpha1.StaticBootstrap{
					InitialCluster: []etcdv1alpha1.InitialClusterMember{
						{
							Name: "bees",
							Host: "bees.my-cluster",
						},
						{
							Name: "magic",
							Host: "magic.my-cluster",
						},
						{
							Name: "goose",
							Host: "goose.my-cluster",
						},
					},
				},
				InitialClusterState: etcdv1alpha1.InitialClusterStateNew,
			},
			Storage: &etcdv1alpha1.EtcdPeerStorage{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: pointer.StringPtr("standard"),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": resource.MustParse("1Mi"),
						},
					},
				},
			},
			PodTemplate: &etcdv1alpha1.EtcdPodTemplateSpec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						"cpu":    resource.MustParse("200m"),
						"memory": resource.MustParse("200Mi"),
					},
					Limits: corev1.ResourceList{
						"cpu":    resource.MustParse("200m"),
						"memory": resource.MustParse("200Mi"),
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
				Source: etcdv1alpha1.EtcdBackupSource{
					ClusterURL: "",
				},
				Destination: etcdv1alpha1.EtcdBackupDestination{
					ObjectURL: "",
				},
			},
		},
	}
}
