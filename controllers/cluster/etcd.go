package cluster

import (
	"fmt"
	"net/url"
	"time"

	etcdclient "go.etcd.io/etcd/client"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

func EtcdClientConfig(cluster *etcdv1alpha1.EtcdCluster) etcdclient.Config {
	serviceURL := &url.URL{
		Scheme: etcdv1alpha1.EtcdScheme,
		// We (the operator) are quite probably in a different namespace to the cluster, so we need to use a fully
		// defined URL.
		Host: fmt.Sprintf("%s.%s.svc:%d", cluster.Name, cluster.Namespace, etcdv1alpha1.EtcdClientPort),
	}
	return etcdclient.Config{
		Endpoints:               []string{serviceURL.String()},
		Transport:               etcdclient.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second * 1,
	}
}
