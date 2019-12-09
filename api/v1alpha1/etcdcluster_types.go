package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EtcdClusterSpec defines the desired state of EtcdCluster
type EtcdClusterSpec struct {
	// Number of instances of etcd to assemble into this cluster
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Minimum=1
	Replicas *int32 `json:"replicas"`

	// Storage is the configuration of the disks and mount points of the Etcd
	// peers.
	Storage *EtcdPeerStorage `json:"storage,omitempty"`

	// PodTemplate describes metadata that should be applied to the underlying Pods. This may not be applied verbatim,
	// as additional metadata may be added by the operator. In particular the operator reserves label and annotation
	// names starting with `etcd.improbable.io`, and pod templates containing these are considered invalid and will be
	// rejected.
	// +optional
	PodTemplate *EtcdPodTemplateSpec `json:"podTemplate,omitempty"`
}

// EtcdPodTemplateSpec supports a subset of a normal `v1/PodTemplateSpec` that the operator explicitly permits. We don't
// want to allow a user to set arbitrary features on our underlying pods.
type EtcdPodTemplateSpec struct {

	// Metadata is elements to be applied to the final metadata of the underlying pods.
	// +optional
	Metadata *EtcdPodTemplateObjectMeta `json:"metadata,omitempty"`
}

// EtcdPodTemplateObjectMeta supports a subset of the features of a normal ObjectMeta. In particular the ones we allow.
type EtcdPodTemplateObjectMeta struct {

	// Annotations are an unstructured string:string map of annotations to be applied to the underlying pods.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

type EtcdMember struct {
	// Name is a human-readable name for the member. Will *typically* match the name we gave the peer that manages this
	// member.
	Name string `json:"name"`

	// ID is the internal unique identifier for the member that defines its identity with the etcd cluster. We do not
	// define this.
	ID string `json:"id"`
}

// EtcdClusterStatus defines the observed state of EtcdCluster
type EtcdClusterStatus struct {
	// Replicas is the number of etcd peer resources we are managing. This doesn't mean the number of pods that exist
	// (as we may have just created a peer resource that doesn't have a pod yet, or the pod could be restarting), and it
	// doesn't mean the number of members the etcd cluster has live, as pods may not be ready yet or network problems
	// may mean the cluster has lost a member.
	Replicas int32 `json:"replicas"`

	// Members contains information about each member from the etcd cluster.
	// +optional
	Members []EtcdMember `json:"members"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas

// EtcdCluster is the Schema for the etcdclusters API
type EtcdCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdClusterSpec   `json:"spec,omitempty"`
	Status EtcdClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdClusterList contains a list of EtcdCluster
type EtcdClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EtcdCluster{}, &EtcdClusterList{})
}
