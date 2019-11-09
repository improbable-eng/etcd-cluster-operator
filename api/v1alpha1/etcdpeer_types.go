package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InitialClusterMemeber describes a single member of the initial cluster.
type InitialClusterMember struct {
	// Name is a friendly name for the peer, used as a means to identify the
	// peer once it has joined a cluster. This should match the `name` field
	// of the `EtcdPeer` resource representing that peer.
	Name string `json:"name"`

	// Host forms part of the Advertise URL - the URL at which this peer can
	// be contacted. The port and scheme are hardcoded to 2380 and http
	// respectively.
	Host string `json:"host"`
}

// StaticBootstrap provides static contact information for initial members of
// the cluster.
type StaticBootstrap struct {
	// InitialCluster provides details of all initial cluster members,
	// and should include ourselves.
	// +kubebuilder:validation:MinItems:=1
	InitialCluster []InitialClusterMember `json:"initialCluster,omitempty"`
}

// Bootstrap contains bootstrap infromation for the peer to use.
type Bootstrap struct {
	// Static boostrapping requires that we know the network names of the
	// other peers ahead of time.
	// +optional
	Static StaticBootstrap `json:"static,omitempty"`
}

// EtcdPeerSpec defines the desired state of EtcdPeer
type EtcdPeerSpec struct {
	// The name of the etcd cluster that this peer should join. This will be
	// used to set the `spec.subdomain` field and the
	// `etcd.improbable.io/cluster-name` label on the Pod running etcd.
	// +kubebuilder:validation:MaxLength:=64
	ClusterName string `json:"clusterName"`

	// Bootstrap is the bootstrap configuration to pass down into the etcd
	// pods. As per the etcd documentation, etcd will ignore bootstrap
	// instructions if it already knows where it's peers are.
	// +optional
	Bootstrap Bootstrap `json:"bootstrap,omitempty"`

	// Storage is the configuration of the disks and mount points of the Etcd
	// pod.
	Storage EtcdPeerStorage `json:"storage"`
}

// EtcdPeerStorage defines the desired storage for an EtcdPeer
type EtcdPeerStorage struct {
	// VolumeClaimTemplates is a claim that pods are allowed to reference.
	// The EtcdPeer controller will create a new PersistentVolumeClaim using the
	// StorageClass and the Storage Resource Request in this template.
	// That PVC will then be mounted in the Pod for this EtcdPeer and the Etcd
	// process when it starts will persist its data to the PV bound to that PVC.
	VolumeClaimTemplate corev1.PersistentVolumeClaimSpec `json:"volumeClaimTemplate"`
}

// EtcdPeerStatus defines the observed state of EtcdPeer
type EtcdPeerStatus struct {
}

// +kubebuilder:object:root=true

// EtcdPeer is the Schema for the etcdpeers API
type EtcdPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdPeerSpec   `json:"spec,omitempty"`
	Status EtcdPeerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdPeerList contains a list of EtcdPeer
type EtcdPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdPeer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdPeer{}, &EtcdPeerList{})
}
