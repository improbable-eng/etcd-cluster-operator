package controllers

const (
	etcdClientPort          = 2379
	etcdPeerPort            = 2380
	EtcdDataMountPath       = "/var/lib/etcd"
	EtcdCertPath            = "/etc/etcd/certs"
	appName                 = "etcd"
	appLabel                = "app.kubernetes.io/name"
	clusterLabel            = "etcd.improbable.io/cluster-name"
	scheduleLabel           = "etcd.improbable.io/backup-schedule-name"
	scheduleCancelFinalizer = "finalizers.etcd.improbable.io/backup-schedule-cancel"
)
