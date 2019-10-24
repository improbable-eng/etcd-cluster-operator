package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	defaultVolumeMode = corev1.PersistentVolumeFilesystem
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
	Static *StaticBootstrap `json:"static,omitempty"`
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
	Bootstrap *Bootstrap `json:"bootstrap,omitempty"`

	// Storage is the configuration of the disks and mount points of the Etcd
	// pod.
	Storage *EtcdPeerStorage `json:"storage,omitempty"`
}

// EtcdPeerStorage defines the desired storage for an EtcdPeer
type EtcdPeerStorage struct {
	// VolumeClaimTemplates is a claim that pods are allowed to reference.
	// The EtcdPeer controller will create a new PersistentVolumeClaim using the
	// StorageClass and the Storage Resource Request in this template.
	// That PVC will then be mounted in the Pod for this EtcdPeer and the Etcd
	// process when it starts will persist its data to the PV bound to that PVC.
	VolumeClaimTemplate *corev1.PersistentVolumeClaimSpec `json:"volumeClaimTemplate,omitempty"`
}

func validatePersistentVolumeClaimSpec(path *field.Path, o *corev1.PersistentVolumeClaimSpec) field.ErrorList {
	var allErrs field.ErrorList
	if o == nil {
		allErrs = append(allErrs, field.Required(path, ""))
		return allErrs
	}
	if o.StorageClassName == nil {
		allErrs = append(allErrs, field.Required(path.Child("storageClassName"), ""))
		return allErrs
	}
	if _, ok := o.Resources.Requests["storage"]; !ok {
		allErrs = append(allErrs, field.Required(path.Child("resources", "requests", "storage"), ""))
		return allErrs
	}
	return allErrs
}

func (o *EtcdPeerStorage) validate(path *field.Path) field.ErrorList {
	var allErrs field.ErrorList
	if o == nil {
		allErrs = append(allErrs, field.Required(path, ""))
		return allErrs
	}
	allErrs = append(
		allErrs,
		validatePersistentVolumeClaimSpec(path.Child("volumeClaimTemplate"), o.VolumeClaimTemplate)...,
	)
	return allErrs
}

func (o *EtcdPeerStorage) setDefaults() {
	if o.VolumeClaimTemplate != nil {
		if o.VolumeClaimTemplate.AccessModes == nil {
			o.VolumeClaimTemplate.AccessModes = []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			}
		}

		if o.VolumeClaimTemplate.VolumeMode == nil {
			o.VolumeClaimTemplate.VolumeMode = &defaultVolumeMode
		}
	}
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

var _ webhook.Validator = &EtcdPeer{}

// ValidateCreate validates that all required fields are present and valid.
func (o *EtcdPeer) ValidateCreate() error {
	path := field.NewPath("spec")
	var allErrs field.ErrorList

	allErrs = append(
		allErrs,
		o.Spec.Storage.validate(path.Child("storage"))...,
	)
	return allErrs.ToAggregate()
}

// ValidateCreate validates that deletion is allowed
// TODO: Not yet implemented
func (o *EtcdPeer) ValidateDelete() error {
	var allErrs field.ErrorList
	return allErrs.ToAggregate()
}

// ValidateCreate validates that only supported fields are changed
// TODO: Not yet implemented
func (o *EtcdPeer) ValidateUpdate(old runtime.Object) error {
	var allErrs field.ErrorList
	return allErrs.ToAggregate()
}

var _ webhook.Defaulter = &EtcdPeer{}

// Default sets default values for optional EtcdPeer fields.
// This is used in webhooks and in the Reconciler to ensure that nil pointers
// have been replaced with concrete pointers.
// This avoids nil pointer panics later on.
func (o *EtcdPeer) Default() {
	if o.Spec.Storage != nil {
		o.Spec.Storage.setDefaults()
	}
}

// ExampleEtcdPeer returns a valid example for testing purposes
func ExampleEtcdPeer(namespace string) *EtcdPeer {
	return &EtcdPeer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bees",
			Namespace: namespace,
		},
		Spec: EtcdPeerSpec{
			ClusterName: "my-cluster",
			Bootstrap: &Bootstrap{
				Static: &StaticBootstrap{
					InitialCluster: []InitialClusterMember{
						{
							Name: "bees",
							Host: fmt.Sprintf("bees.my-cluster.%s.svc", namespace),
						},
						{
							Name: "magic",
							Host: fmt.Sprintf("magic.my-cluster.%s.svc", namespace),
						},
						{
							Name: "goose",
							Host: fmt.Sprintf("goose.my-cluster.%s.svc", namespace),
						},
					},
				},
			},
			Storage: &EtcdPeerStorage{
				VolumeClaimTemplate: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: pointer.StringPtr("example-class"),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": resource.MustParse("999Gi"),
						},
					},
				},
			},
		},
	}
}

func init() {
	SchemeBuilder.Register(&EtcdPeer{}, &EtcdPeerList{})
}
