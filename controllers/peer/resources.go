package peer

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcdenvvar"
)

func pvcForPeer(peer *etcdv1alpha1.EtcdPeer) *corev1.PersistentVolumeClaim {
	labels := map[string]string{
		etcdv1alpha1.AppLabel:     etcdv1alpha1.AppName,
		etcdv1alpha1.ClusterLabel: peer.Spec.ClusterName,
		etcdv1alpha1.PeerLabel:    peer.Name,
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peer.Name,
			Namespace: peer.Namespace,
			Labels:    labels,
		},
		Spec: *peer.Spec.Storage.VolumeClaimTemplate.DeepCopy(),
	}
}

func initialMemberURL(member etcdv1alpha1.InitialClusterMember) *url.URL {
	return &url.URL{
		Scheme: etcdv1alpha1.EtcdScheme,
		Host:   fmt.Sprintf("%s:%d", member.Host, etcdv1alpha1.EtcdPeerPort),
	}
}

// staticBootstrapInitialCluster returns the value of `ETCD_INITIAL_CLUSTER`
// environment variable.
func staticBootstrapInitialCluster(static etcdv1alpha1.StaticBootstrap) string {
	s := make([]string, len(static.InitialCluster))
	// Put our peers in as the other entries
	for i, member := range static.InitialCluster {
		s[i] = fmt.Sprintf("%s=%s",
			member.Name,
			initialMemberURL(member).String())
	}
	return strings.Join(s, ",")
}

// advertiseURL builds the canonical URL of this peer from it's name and the
// cluster name.
func advertiseURL(etcdPeer etcdv1alpha1.EtcdPeer, port int32) *url.URL {
	return &url.URL{
		Scheme: etcdv1alpha1.EtcdScheme,
		Host: fmt.Sprintf(
			"%s.%s.%s.svc:%d",
			etcdPeer.Name,
			etcdPeer.Spec.ClusterName,
			etcdPeer.Namespace,
			port,
		),
	}
}

func bindAllAddress(port int) *url.URL {
	return &url.URL{
		Scheme: etcdv1alpha1.EtcdScheme,
		Host:   fmt.Sprintf("0.0.0.0:%d", port),
	}
}

func clusterStateValue(cs etcdv1alpha1.InitialClusterState) string {
	if cs == etcdv1alpha1.InitialClusterStateNew {
		return "new"
	} else if cs == etcdv1alpha1.InitialClusterStateExisting {
		return "existing"
	} else {
		return ""
	}
}

// goMaxProcs calculates an appropriate Golang thread limit (GOMAXPROCS) for the
// configured CPU limit.
//
// GOMAXPROCS defaults to the number of CPUs on the Kubelet host which may be
// much higher than the requests and limits defined for the pod,
// See https://github.com/golang/go/issues/33803
// If resources have been set and if CPU limit is > 0 then set GOMAXPROCS to an
// integer between 1 and floor(cpuLimit).
// Etcd might one day set its own GOMAXPROCS based on CPU quota:
// See: https://github.com/etcd-io/etcd/issues/11508
func goMaxProcs(cpuLimit resource.Quantity) *int64 {
	switch cpuLimit.Sign() {
	case -1, 0:
		return nil
	}
	goMaxProcs := cpuLimit.MilliValue() / 1000
	if goMaxProcs < 1 {
		goMaxProcs = 1
	}
	return pointer.Int64Ptr(goMaxProcs)
}

func defineReplicaSet(peer *etcdv1alpha1.EtcdPeer, log logr.Logger) *appsv1.ReplicaSet {
	var replicas int32 = 1

	// We use the same labels for the replica set itself, the selector on
	// the replica set, and the pod template under the replica set.
	labels := map[string]string{
		etcdv1alpha1.AppLabel:     etcdv1alpha1.AppName,
		etcdv1alpha1.ClusterLabel: peer.Spec.ClusterName,
		etcdv1alpha1.PeerLabel:    peer.Name,
	}

	etcdContainer := corev1.Container{
		Name:  etcdv1alpha1.AppName,
		Image: etcdv1alpha1.EtcdImage,
		Env: []corev1.EnvVar{
			{
				Name:  etcdenvvar.InitialCluster,
				Value: staticBootstrapInitialCluster(*peer.Spec.Bootstrap.Static),
			},
			{
				Name:  etcdenvvar.Name,
				Value: peer.Name,
			},
			{
				Name:  etcdenvvar.InitialClusterToken,
				Value: peer.Spec.ClusterName,
			},
			{
				Name:  etcdenvvar.InitialAdvertisePeerURLs,
				Value: advertiseURL(*peer, etcdv1alpha1.EtcdPeerPort).String(),
			},
			{
				Name:  etcdenvvar.AdvertiseClientURLs,
				Value: advertiseURL(*peer, etcdv1alpha1.EtcdClientPort).String(),
			},
			{
				Name:  etcdenvvar.ListenPeerURLs,
				Value: bindAllAddress(etcdv1alpha1.EtcdPeerPort).String(),
			},
			{
				Name:  etcdenvvar.ListenClientURLs,
				Value: bindAllAddress(etcdv1alpha1.EtcdClientPort).String(),
			},
			{
				Name:  etcdenvvar.InitialClusterState,
				Value: clusterStateValue(peer.Spec.Bootstrap.InitialClusterState),
			},
			{
				Name:  etcdenvvar.DataDir,
				Value: etcdv1alpha1.EtcdDataMountPath,
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "etcd-client",
				ContainerPort: etcdv1alpha1.EtcdClientPort,
			},
			{
				Name:          "etcd-peer",
				ContainerPort: etcdv1alpha1.EtcdPeerPort,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "etcd-data",
				MountPath: etcdv1alpha1.EtcdDataMountPath,
			},
		},
	}
	if peer.Spec.PodTemplate != nil {
		if peer.Spec.PodTemplate.Resources != nil {
			etcdContainer.Resources = *peer.Spec.PodTemplate.Resources.DeepCopy()
			if value := goMaxProcs(*etcdContainer.Resources.Limits.Cpu()); value != nil {
				etcdContainer.Env = append(
					etcdContainer.Env,
					corev1.EnvVar{
						Name:  "GOMAXPROCS",
						Value: fmt.Sprintf("%d", *value),
					},
				)
			}
		}
	}
	replicaSet := appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Annotations:     make(map[string]string),
			Name:            peer.Name,
			Namespace:       peer.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(peer, etcdv1alpha1.GroupVersion.WithKind("EtcdPeer"))},
		},
		Spec: appsv1.ReplicaSetSpec{
			// This will *always* be 1. Other peers are handled by other EtcdPeers.
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: make(map[string]string),
					Name:        peer.Name,
					Namespace:   peer.Namespace,
				},
				Spec: corev1.PodSpec{
					Hostname:   peer.Name,
					Subdomain:  peer.Spec.ClusterName,
					Containers: []corev1.Container{etcdContainer},
					Volumes: []corev1.Volume{
						{
							Name: "etcd-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: peer.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	if peer.Spec.PodTemplate != nil {
		if peer.Spec.PodTemplate.Metadata != nil {
			// Stamp annotations
			for name, value := range peer.Spec.PodTemplate.Metadata.Annotations {
				if !etcdv1alpha1.IsInvalidUserProvidedAnnotationName(name) {
					if _, found := replicaSet.Spec.Template.Annotations[name]; !found {
						replicaSet.Spec.Template.Annotations[name] = value
					} else {
						// This will only check against an annotation that we set ourselves.
						log.V(2).Info("Ignoring annotation, we already have one with that name",
							"annotation-name", name)
					}
				} else {
					// In theory, this code is unreachable as we check this validation at the start of the reconcile
					// loop. See https://xkcd.com/2200
					log.V(2).Info("Ignoring annotation, applying etcd.improbable.io/ annotations is not supported",
						"annotation-name", name)
				}
			}
		}
	}

	return &replicaSet
}
