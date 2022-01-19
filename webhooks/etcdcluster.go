package webhooks

import (
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
)

// EtcdCluster validating webhook
//+kubebuilder:webhook:path=/validate-etcd-improbable-io-v1alpha1-etcdcluster,verbs=create;update,mutating=false,failurePolicy=fail,groups=etcd.improbable.io,resources=etcdclusters,versions=v1alpha1,name=validation.etcdclusters.etcd.improbable.io,sideEffects=none,admissionReviewVersions=([]string)
// EtcdCluster defaulting webhook
//+kubebuilder:webhook:path=/mutate-etcd-improbable-io-v1alpha1-etcdcluster,verbs=create;update,mutating=true,failurePolicy=fail,groups=etcd.improbable.io,resources=etcdclusters,versions=v1alpha1,name=default.etcdclusters.etcd.improbable.io,sideEffects=none,admissionReviewVersions=([]string)

// EtcdPeer validating webhook
//+kubebuilder:webhook:path=/validate-etcd-improbable-io-v1alpha1-etcdpeer,verbs=create;update,mutating=false,failurePolicy=fail,groups=etcd.improbable.io,resources=etcdpeers,versions=v1alpha1,name=validation.etcdpeers.etcd.improbable.io,sideEffects=none,admissionReviewVersions=([]string)

// EtcdPeer defaulting webhook
//+kubebuilder:webhook:path=/mutate-etcd-improbable-io-v1alpha1-etcdpeer,verbs=create;update,mutating=true,failurePolicy=fail,groups=etcd.improbable.io,resources=etcdpeers,versions=v1alpha1,name=default.etcdpeers.etcd.improbable.io,sideEffects=none,admissionReviewVersions=([]string)

type EtcdCluster struct {
	Log logr.Logger
}

func (o *EtcdCluster) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.EtcdCluster{}).
		Complete()
}

type EtcdPeer struct {
	Log logr.Logger
}

func (o *EtcdPeer) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.EtcdPeer{}).
		Complete()
}
