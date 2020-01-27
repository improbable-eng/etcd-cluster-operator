package cluster

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

func serviceName(cluster *etcdv1alpha1.EtcdCluster) types.NamespacedName {
	return types.NamespacedName{
		Name:      cluster.Name,
		Namespace: cluster.Namespace,
	}
}

func headlessServiceForCluster(cluster *etcdv1alpha1.EtcdCluster) *v1.Service {
	name := serviceName(cluster)
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(cluster, etcdv1alpha1.GroupVersion.WithKind("EtcdCluster")),
			},
			Labels: map[string]string{
				etcdv1alpha1.AppLabel:     etcdv1alpha1.AppName,
				etcdv1alpha1.ClusterLabel: cluster.Name,
			},
		},
		Spec: v1.ServiceSpec{
			ClusterIP:                v1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Selector: map[string]string{
				etcdv1alpha1.AppLabel:     etcdv1alpha1.AppName,
				etcdv1alpha1.ClusterLabel: cluster.Name,
			},
			Ports: []v1.ServicePort{
				{
					Name:     "etcd-client",
					Protocol: "TCP",
					Port:     etcdv1alpha1.EtcdClientPort,
				},
				{
					Name:     "etcd-peer",
					Protocol: "TCP",
					Port:     etcdv1alpha1.EtcdPeerPort,
				},
			},
		},
	}
}
