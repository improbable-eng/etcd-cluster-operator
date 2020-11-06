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
	CertFile                 = "ETCD_CERT_FILE"
	KeyFile                  = "ETCD_KEY_FILE"
	TrustedCaFile            = "ETCD_TRUSTED_CA_FILE"
	ClientCertAuth           = "ETCD_CLIENT_CERT_AUTH"
	PeerCertFile             = "ETCD_PEER_CERT_FILE"
	PeerKeyFile              = "ETCD_PEER_KEY_FILE"
	PeerTrustedCaFile        = "ETCD_PEER_TRUSTED_CA_FILE"
	PeerClientCertAuth       = "ETCD_PEER_CLIENT_CERT_AUTH"
	CtlCertFile              = "ETCDCTL_CERT"
	CtlKeyFile               = "ETCDCTL_KEY"
	CtlCaFile                = "ETCDCTL_CACERT"
)
