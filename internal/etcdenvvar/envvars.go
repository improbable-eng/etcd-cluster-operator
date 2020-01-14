package etcdenvvar

const (
	InitialAdvertisePeerURLs = "ETCD_INITIAL_ADVERTISE_PEER_URLS"
	AdvertiseClientURLs      = "ETCD_ADVERTISE_CLIENT_URLS"
	ListenPeerURLs           = "ETCD_LISTEN_PEER_URLS"
	InitialCluster           = "ETCD_INITIAL_CLUSTER"
	ListenClientURLs         = "ETCD_LISTEN_CLIENT_URLS"
	InitialClusterState      = "ETCD_INITIAL_CLUSTER_STATE"
	InitialClusterToken      = "ETCD_INITIAL_CLUSTER_TOKEN"
	Name                     = "ETCD_NAME"
	DataDir                  = "ETCD_DATA_DIR"
	AutoCompactionRetention  = "ETCD_AUTO_COMPACTION_RETENTION"
)
