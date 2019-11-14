package controllers

const (
	etcdClientPort = 2379
	etcdPeerPort   = 2380
	// etcdMetricsPort is the port at which to obtain etcd metrics and health status
	etcdMetricsPort   = 2381
	etcdDataMountPath = "/var/lib/etcd"
	appName           = "etcd"
	appLabel          = "app.kubernetes.io/name"
	clusterLabel      = "etcd.improbable.io/cluster-name"
)
