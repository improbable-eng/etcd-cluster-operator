package v1alpha1

const (
	AppLabel                = "app.kubernetes.io/name"
	AppName                 = "etcd"
	ClusterLabel            = "etcd.improbable.io/cluster-name"
	EtcdClientPort          = 2379
	EtcdDataMountPath       = "/var/lib/etcd"
	EtcdImage               = "quay.io/coreos/etcd:v3.2.28"
	EtcdPeerPort            = 2380
	EtcdScheme              = "http"
	PVCCleanupFinalizer     = "etcdpeer.etcd.improbable.io/pvc-cleanup"
	PeerLabel               = "etcd.improbable.io/peer-name"
	ScheduleCancelFinalizer = "finalizers.etcd.improbable.io/backup-schedule-cancel"
	ScheduleLabel           = "etcd.improbable.io/backup-schedule-name"
)
