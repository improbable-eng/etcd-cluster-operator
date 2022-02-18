package controllers

import (
	"context"
	cryptotls "crypto/tls"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcdenvvar"
	"github.com/improbable-eng/etcd-cluster-operator/internal/tls"
)

// EtcdPeerReconciler reconciles a EtcdPeer object
type EtcdPeerReconciler struct {
	client.Client
	Log            logr.Logger
	EtcdRepository string
	Etcd           etcd.APIBuilder
}

const (
	peerLabel           = "etcd.improbable.io/peer-name"
	pvcCleanupFinalizer = "etcdpeer.etcd.improbable.io/pvc-cleanup"
)

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdpeers/status;etcdpeers/finalizers,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=list;get;create;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=list;get;create;watch;delete

func initialMemberURL(member etcdv1alpha1.InitialClusterMember, etcdScheme string) *url.URL {
	return &url.URL{
		Scheme: etcdScheme,
		Host:   fmt.Sprintf("%s:%d", member.Host, etcdPeerPort),
	}
}

// staticBootstrapInitialCluster returns the value of `ETCD_INITIAL_CLUSTER`
// environment variable.
func staticBootstrapInitialCluster(static etcdv1alpha1.StaticBootstrap, etcdScheme string) string {
	s := make([]string, len(static.InitialCluster))
	// Put our peers in as the other entries
	for i, member := range static.InitialCluster {
		s[i] = fmt.Sprintf("%s=%s",
			member.Name,
			initialMemberURL(member, etcdScheme).String())
	}
	return strings.Join(s, ",")
}

// advertiseURL builds the canonical URL of this peer from it's name and the
// cluster name.
func advertiseURL(etcdPeer etcdv1alpha1.EtcdPeer, port int32) *url.URL {
	return &url.URL{
		Scheme: etcdScheme(etcdPeer.Spec.TLS),
		Host: fmt.Sprintf(
			"%s.%s:%d",
			etcdPeer.Name,
			etcdPeer.Spec.ClusterName,
			port,
		),
	}
}

// advertiseClientURL builds the canonical URL of this peer from it's name and the
// cluster name.
func advertiseClientURL(etcdPeer etcdv1alpha1.EtcdPeer, port int32) *url.URL {
	return &url.URL{
		Scheme: etcdScheme(etcdPeer.Spec.TLS),
		Host: fmt.Sprintf(
			"%s.%s.%s:%d",
			etcdPeer.Name,
			etcdPeer.Spec.ClusterName,
			etcdPeer.Namespace,
			port,
		),
	}
}

func bindAllAddress(port int, etcdScheme string) *url.URL {
	return &url.URL{
		Scheme: etcdScheme,
		Host:   fmt.Sprintf("0.0.0.0:%d", port),
	}
}

func clusterStateValue(cs etcdv1alpha1.InitialClusterState) string {
	if cs == etcdv1alpha1.InitialClusterStateNew {
		return "new"
	} else if cs == etcdv1alpha1.InitialClusterStateExisting {
		return "existing"
	} else {
		return ""
	}
}

// goMaxProcs calculates an appropriate Golang thread limit (GOMAXPROCS) for the
// configured CPU limit.
//
// GOMAXPROCS defaults to the number of CPUs on the Kubelet host which may be
// much higher than the requests and limits defined for the pod,
// See https://github.com/golang/go/issues/33803
// If resources have been set and if CPU limit is > 0 then set GOMAXPROCS to an
// integer between 1 and floor(cpuLimit).
// Etcd might one day set its own GOMAXPROCS based on CPU quota:
// See: https://github.com/etcd-io/etcd/issues/11508
func goMaxProcs(cpuLimit resource.Quantity) *int64 {
	switch cpuLimit.Sign() {
	case -1, 0:
		return nil
	}
	goMaxProcs := cpuLimit.MilliValue() / 1000
	if goMaxProcs < 1 {
		goMaxProcs = 1
	}
	return pointer.Int64Ptr(goMaxProcs)
}
func definePeerCert(peer etcdv1alpha1.EtcdPeer, caCert, cert, key []byte) corev1.Secret {
	// We use the same labels for the replica set itself, the selector on
	// the replica set, and the pod template under the replica set.
	labels := map[string]string{
		appLabel:     appName,
		clusterLabel: peer.Spec.ClusterName,
		peerLabel:    peer.Name,
	}

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Annotations:     make(map[string]string),
			Name:            peer.Name,
			Namespace:       peer.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(&peer, etcdv1alpha1.GroupVersion.WithKind("EtcdPeer"))},
		},
		Data: make(map[string][]byte),
	}

	secret.Data["ca.crt"] = caCert
	secret.Data["tls.crt"] = cert
	secret.Data["tls.key"] = key

	return secret
}

func defineReplicaSet(peer etcdv1alpha1.EtcdPeer, etcdRepository string, log logr.Logger) appsv1.ReplicaSet {
	var replicas int32 = 1

	// We use the same labels for the replica set itself, the selector on
	// the replica set, and the pod template under the replica set.
	labels := map[string]string{
		appLabel:     appName,
		clusterLabel: peer.Spec.ClusterName,
		peerLabel:    peer.Name,
	}

	etcdContainer := corev1.Container{
		Name:  appName,
		Image: fmt.Sprintf("%s:v%s", etcdRepository, peer.Spec.Version),
		Env: []corev1.EnvVar{
			{
				Name:  etcdenvvar.InitialCluster,
				Value: staticBootstrapInitialCluster(*peer.Spec.Bootstrap.Static, etcdScheme(peer.Spec.TLS)),
			},
			{
				Name:  etcdenvvar.Name,
				Value: peer.Name,
			},
			{
				Name:  etcdenvvar.InitialClusterToken,
				Value: peer.Spec.ClusterName,
			},
			{
				Name:  etcdenvvar.InitialAdvertisePeerURLs,
				Value: advertiseURL(peer, etcdPeerPort).String(),
			},
			{
				Name:  etcdenvvar.AdvertiseClientURLs,
				Value: advertiseClientURL(peer, etcdClientPort).String(),
			},
			{
				Name:  etcdenvvar.ListenPeerURLs,
				Value: bindAllAddress(etcdPeerPort, etcdScheme(peer.Spec.TLS)).String(),
			},
			{
				Name:  etcdenvvar.ListenClientURLs,
				Value: bindAllAddress(etcdClientPort, etcdScheme(peer.Spec.TLS)).String(),
			},
			{
				Name:  etcdenvvar.InitialClusterState,
				Value: clusterStateValue(peer.Spec.Bootstrap.InitialClusterState),
			},
			{
				Name:  etcdenvvar.DataDir,
				Value: EtcdDataMountPath,
			},
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          "etcd-client",
				ContainerPort: etcdClientPort,
			},
			{
				Name:          "etcd-peer",
				ContainerPort: etcdPeerPort,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "etcd-data",
				MountPath: EtcdDataMountPath,
			},
		},
	}

	if peer.Spec.TLS != nil && peer.Spec.TLS.Enabled {
		etcdContainer.Env = append(
			etcdContainer.Env,
			corev1.EnvVar{
				Name:  etcdenvvar.PeerTrustedCaFile,
				Value: fmt.Sprintf("%s/ca.crt", EtcdCertPath),
			},
			corev1.EnvVar{
				Name:  etcdenvvar.PeerCertFile,
				Value: fmt.Sprintf("%s/tls.crt", EtcdCertPath),
			},
			corev1.EnvVar{
				Name:  etcdenvvar.PeerKeyFile,
				Value: fmt.Sprintf("%s/tls.key", EtcdCertPath),
			},
			corev1.EnvVar{
				Name:  etcdenvvar.PeerClientCertAuth,
				Value: "true",
			},
			corev1.EnvVar{
				Name:  etcdenvvar.TrustedCaFile,
				Value: fmt.Sprintf("%s/ca.crt", EtcdCertPath),
			},
			corev1.EnvVar{
				Name:  etcdenvvar.CertFile,
				Value: fmt.Sprintf("%s/tls.crt", EtcdCertPath),
			},
			corev1.EnvVar{
				Name:  etcdenvvar.KeyFile,
				Value: fmt.Sprintf("%s/tls.key", EtcdCertPath),
			},
			corev1.EnvVar{
				Name:  etcdenvvar.ClientCertAuth,
				Value: "true",
			},
			corev1.EnvVar{
				Name:  etcdenvvar.CtlCaFile,
				Value: fmt.Sprintf("%s/ca.crt", EtcdCertPath),
			},
			corev1.EnvVar{
				Name:  etcdenvvar.CtlCertFile,
				Value: fmt.Sprintf("%s/tls.crt", EtcdCertPath),
			},
			corev1.EnvVar{
				Name:  etcdenvvar.CtlKeyFile,
				Value: fmt.Sprintf("%s/tls.key", EtcdCertPath),
			},
		)
		etcdContainer.VolumeMounts = append(
			etcdContainer.VolumeMounts,
			corev1.VolumeMount{
				Name:      "cert-dir",
				MountPath: EtcdCertPath,
			},
		)
	}

	if peer.Spec.PodTemplate != nil {
		if peer.Spec.PodTemplate.Resources != nil {
			etcdContainer.Resources = *peer.Spec.PodTemplate.Resources.DeepCopy()
			if value := goMaxProcs(*etcdContainer.Resources.Limits.Cpu()); value != nil {
				etcdContainer.Env = append(
					etcdContainer.Env,
					corev1.EnvVar{
						Name:  "GOMAXPROCS",
						Value: fmt.Sprintf("%d", *value),
					},
				)
			}
		}
	}
	// Append additional env vars that were passed directly to etcdcluster and overwrite any duplicates
	if peer.Spec.PodTemplate != nil {
		etcdContainer.Env = appendClusterEnvVars(etcdContainer.Env, peer.Spec.PodTemplate.EtcdEnv)
	}

	replicaSet := appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          labels,
			Annotations:     make(map[string]string),
			Name:            peer.Name,
			Namespace:       peer.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(&peer, etcdv1alpha1.GroupVersion.WithKind("EtcdPeer"))},
		},
		Spec: appsv1.ReplicaSetSpec{
			// This will *always* be 1. Other peers are handled by other EtcdPeers.
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: make(map[string]string),
					Name:        peer.Name,
					Namespace:   peer.Namespace,
				},
				Spec: corev1.PodSpec{
					Hostname:  peer.Name,
					Subdomain: peer.Spec.ClusterName,
					HostAliases: []corev1.HostAlias{
						{
							IP: "127.0.0.1",
							Hostnames: []string{
								fmt.Sprintf("%s.%s", peer.Name, peer.Spec.ClusterName),
							},
						},
					},
					Containers: []corev1.Container{etcdContainer},
					Volumes: []corev1.Volume{
						{
							Name: "etcd-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: peer.Name,
								},
							},
						},
					},
				},
			},
		},
	}

	if peer.Spec.TLS != nil && peer.Spec.TLS.Enabled {
		certVolume := corev1.Volume{
			Name: "cert-dir",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: peer.Name,
				},
			},
		}
		replicaSet.Spec.Template.Spec.Volumes = append(replicaSet.Spec.Template.Spec.Volumes, certVolume)
	}

	if peer.Spec.PodTemplate != nil {
		if peer.Spec.PodTemplate.Metadata != nil {
			// Stamp annotations
			for name, value := range peer.Spec.PodTemplate.Metadata.Annotations {
				if !etcdv1alpha1.IsInvalidUserProvidedAnnotationName(name) {
					if _, found := replicaSet.Spec.Template.Annotations[name]; !found {
						replicaSet.Spec.Template.Annotations[name] = value
					} else {
						// This will only check against an annotation that we set ourselves.
						log.V(2).Info("Ignoring annotation, we already have one with that name",
							"annotation-name", name)
					}
				} else {
					// In theory, this code is unreachable as we check this validation at the start of the reconcile
					// loop. See https://xkcd.com/2200
					log.V(2).Info("Ignoring annotation, applying etcd.improbable.io/ annotations is not supported",
						"annotation-name", name)
				}
			}
		}
		if peer.Spec.PodTemplate.Affinity != nil {
			replicaSet.Spec.Template.Spec.Affinity = peer.Spec.PodTemplate.Affinity
		}
		if len(peer.Spec.PodTemplate.Tolerations) > 0 {
			replicaSet.Spec.Template.Spec.Tolerations = peer.Spec.PodTemplate.Tolerations
		}
	}

	return replicaSet
}

// appendClusterEnvVars adds EnvVars passed via etcdcluster spec to EnvVars for peer etcd container.
// Any EnvVars duplicated in the etcdcluster spec overwrite those created by default by this controller.
func appendClusterEnvVars(peerEnvs, clusterEnvs []corev1.EnvVar) []corev1.EnvVar {
	if len(clusterEnvs) == 0 {
		return peerEnvs
	}
	for i, peerEnv := range peerEnvs {
		for j, clusterEnv := range clusterEnvs {
			if peerEnv.Name == clusterEnv.Name {
				peerEnvs[i] = clusterEnv
				clusterEnvs = append(clusterEnvs[:j], clusterEnvs[j+1:]...)
				break
			}
		}

	}
	return append(peerEnvs, clusterEnvs...)
}

func pvcForPeer(peer *etcdv1alpha1.EtcdPeer) *corev1.PersistentVolumeClaim {
	labels := map[string]string{
		appLabel:     appName,
		clusterLabel: peer.Spec.ClusterName,
		peerLabel:    peer.Name,
	}

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      peer.Name,
			Namespace: peer.Namespace,
			Labels:    labels,
		},
		Spec: *peer.Spec.Storage.VolumeClaimTemplate.DeepCopy(),
	}
}

func (r *EtcdPeerReconciler) maybeCreatePvc(ctx context.Context, peer *etcdv1alpha1.EtcdPeer) (created bool, err error) {
	objectKey := client.ObjectKey{
		Name:      peer.Name,
		Namespace: peer.Namespace,
	}
	// Check for existing object
	pvc := &corev1.PersistentVolumeClaim{}
	err = r.Get(ctx, objectKey, pvc)
	// Object exists
	if err == nil {
		return false, nil
	}
	// Error when fetching the object
	if !apierrs.IsNotFound(err) {
		return false, err
	}
	// Object does not exist
	err = r.Create(ctx, pvcForPeer(peer))
	// Maybe a stale cache.
	if apierrs.IsAlreadyExists(err) {
		return false, fmt.Errorf("stale cache error: object was not found in cache but creation failed with AlreadyExists error: %w", err)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// getCaCertificates read the ca certificate and key
func (r *EtcdPeerReconciler) getCaCertificates(ctx context.Context, clusterName, namespace string) ([]byte, []byte, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, caSecretName(clusterName, namespace), secret)
	if err != nil {
		// Unexpected error, some other problem?
		return nil, nil, err
	}

	// We found it because we got no error
	return secret.Data["tls.crt"], secret.Data["tls.key"], nil
}

// getClientSecret determines if the client secret exists. Error is returned if there is some problem communicating with
// Kubernetes.
func (r *EtcdPeerReconciler) getClientSecret(ctx context.Context, clusterName, namespace string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, clientSecretName(clusterName, namespace), secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// We got the expected error, which is that it's not found
			return nil, nil
		}
		// Unexpected error, some other problem?
		return nil, err
	}
	// We found it because we got no error
	return secret, nil
}

func hasPvcDeletionFinalizer(peer etcdv1alpha1.EtcdPeer) bool {
	return sets.NewString(peer.ObjectMeta.Finalizers...).Has(pvcCleanupFinalizer)
}

// PeerPVCDeleter deletes the PVC for an EtcdPeer and removes the PVC deletion
// finalizer.
type PeerPVCDeleter struct {
	log    logr.Logger
	client client.Client
	peer   *etcdv1alpha1.EtcdPeer
}

// Execute performs the deletiong and finalizer removal
func (o *PeerPVCDeleter) Execute(ctx context.Context) error {
	o.log.V(2).Info("Deleting PVC for peer prior to deletion")
	expectedPvc := pvcForPeer(o.peer)
	expectedPvcNamespacedName, err := client.ObjectKeyFromObject(expectedPvc)
	if err != nil {
		return fmt.Errorf("unable to get ObjectKey from PVC: %s", err)
	}
	var actualPvc corev1.PersistentVolumeClaim
	err = o.client.Get(ctx, expectedPvcNamespacedName, &actualPvc)
	switch {
	case err == nil:
		// PVC exists.
		// Check whether it has already been deleted (probably by us).
		// It won't actually be deleted until the garbage collector
		// deletes the Pod which is using it.
		if actualPvc.ObjectMeta.DeletionTimestamp.IsZero() {
			o.log.V(2).Info("Deleting PVC for peer")
			err := o.client.Delete(ctx, expectedPvc)
			if err == nil {
				o.log.V(2).Info("Deleted PVC for peer")
				return nil
			}
			return fmt.Errorf("failed to delete PVC for peer: %w", err)
		}
		o.log.V(2).Info("PVC for peer has already been marked for deletion")

	case apierrors.IsNotFound(err):
		o.log.V(2).Info("PVC not found for peer. Already deleted or never created.")

	case err != nil:
		return fmt.Errorf("failed to get PVC for deleted peer: %w", err)

	}

	// If we reach this stage, the PVC has been deleted or didn't need
	// deleting.
	// Remove the finalizer so that the EtcdPeer can be garbage
	// collected along with its replicaset, pod...and with that the PVC
	// will finally be deleted by the garbage collector.
	o.log.V(2).Info("Removing PVC cleanup finalizer")
	updated := o.peer.DeepCopy()
	controllerutil.RemoveFinalizer(updated, pvcCleanupFinalizer)
	if err := o.client.Patch(ctx, updated, client.MergeFrom(o.peer)); err != nil {
		return fmt.Errorf("failed to remove PVC cleanup finalizer: %w", err)
	}
	o.log.V(2).Info("Removed PVC cleanup finalizer")
	return nil
}

func (r *EtcdPeerReconciler) updateStatus(peer *etcdv1alpha1.EtcdPeer, serverVersion string) {
	peer.Status.ServerVersion = serverVersion
}

func (r *EtcdPeerReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log := r.Log.WithValues("peer", req.NamespacedName)

	var peer etcdv1alpha1.EtcdPeer
	if err := r.Get(ctx, req.NamespacedName, &peer); err != nil {
		// NotFound errors occur when the EtcdPeer has been deleted but a PVC is
		// left behind.
		// Ignore these and do not requeue in this case.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.V(2).Info("Found EtcdPeer resource")

	// Apply defaults in case a defaulting webhook has not been deployed.
	peer.Default()

	// Validate in case a validating webhook has not been deployed
	err := peer.ValidateCreate()
	if err != nil {
		log.Error(err, "invalid EtcdPeer")
		return ctrl.Result{}, nil
	}

	original := peer.DeepCopy()

	// Always attempt to patch the status after each reconciliation.
	defer func() {
		if reflect.DeepEqual(original.Status, peer.Status) {
			return
		}
		if err := r.Client.Status().Patch(ctx, &peer, client.MergeFrom(original)); err != nil {
			reterr = kerrors.NewAggregate([]error{reterr, fmt.Errorf("error while patching EtcdPeer.Status: %s ", err)})
		}
	}()

	// Attempt to dial the etcd cluster, recording the cluster response if we can
	var (
		serverVersion string
		tlsConfig     *cryptotls.Config
	)

	if peer.Spec.TLS != nil && peer.Spec.TLS.Enabled {
		clientSecret, err := r.getClientSecret(ctx, peer.Spec.ClusterName, peer.Namespace)
		if err != nil {
			log.Error(err, "can not get client certificates")
			return ctrl.Result{}, nil
		}

		tlsConfig, err = createEtcdTLSConfig(clientSecret)
		if err != nil {
			log.Error(err, "can not create etcd transport")
			return ctrl.Result{}, nil
		}
	} else {
		tlsConfig = nil
	}

	etcdConfig := etcd.Config{
		Endpoints: []string{
			fmt.Sprintf("%s://%s.%s.%s:%d", etcdScheme(peer.Spec.TLS), peer.Name, peer.Spec.ClusterName, peer.Namespace, etcdClientPort),
		},
		TLS: tlsConfig,
	}

	if c, err := r.Etcd.New(etcdConfig); err != nil {
		log.Error(err, "Unable to connect to etcd")
	} else {
		defer c.Close()
		if version, err := c.GetVersion(ctx); err != nil {
			log.Error(err, "Unable to get Etcd version", "error", err)
		} else {
			serverVersion = version.Server
		}
	}

	r.updateStatus(&peer, serverVersion)

	// Always requeue after ten seconds, as we don't watch on the membership list. So we don't auto-detect changes made
	// to the etcd membership API.
	// TODO(#76) Implement custom watch on etcd membership API, and remove this `requeueAfter`
	result := ctrl.Result{RequeueAfter: time.Second * 10}

	// Check if the peer has been marked for deletion
	if !peer.ObjectMeta.DeletionTimestamp.IsZero() {
		if hasPvcDeletionFinalizer(peer) {
			action := &PeerPVCDeleter{
				log:    log,
				client: r.Client,
				peer:   &peer,
			}
			err := action.Execute(ctx)
			return result, err
		}
		return result, nil
	}

	created, err := r.maybeCreatePvc(ctx, &peer)
	if err != nil || created {
		return result, err
	}

	if peer.Spec.TLS != nil && peer.Spec.TLS.Enabled {
		var peerCert corev1.Secret
		err = r.Get(
			ctx,
			client.ObjectKey{
				Namespace: peer.Namespace,
				Name:      peer.Name,
			},
			&peerCert,
		)
		if apierrs.IsNotFound(err) {
			caCert, caKey, err := r.getCaCertificates(ctx, peer.Spec.ClusterName, peer.Namespace)
			if err != nil {
				return result, err
			}

			hostnames := []string{
				// cluster names
				fmt.Sprintf("%s", peer.Spec.ClusterName),
				fmt.Sprintf("%s.%s", peer.Spec.ClusterName, peer.Namespace),
				fmt.Sprintf("%s.%s.svc", peer.Spec.ClusterName, peer.Namespace),
				fmt.Sprintf("%s.%s.svc.cluster.local", peer.Spec.ClusterName, peer.Namespace),

				// peer names
				fmt.Sprintf("%s.%s", peer.Name, peer.Spec.ClusterName),
				fmt.Sprintf("%s.%s.%s", peer.Name, peer.Spec.ClusterName, peer.Namespace),
				fmt.Sprintf("%s.%s.%s.svc", peer.Name, peer.Spec.ClusterName, peer.Namespace),
				fmt.Sprintf("%s.%s.%s.svc.cluster.local", peer.Name, peer.Spec.ClusterName, peer.Namespace),
			}
			cert, key, err := tls.Issue(hostnames, caCert, caKey)
			if err != nil {
				return result, err
			}

			peerCert = definePeerCert(peer, caCert, cert, key)
			if err := r.Create(ctx, &peerCert); err != nil {
				log.Error(err, "unable to create certificate for EtcdPeer", "peer-cert", peerCert)
				return result, err
			}
		}
	}

	var existingReplicaSet appsv1.ReplicaSet
	err = r.Get(
		ctx,
		client.ObjectKey{
			Namespace: peer.Namespace,
			Name:      peer.Name,
		},
		&existingReplicaSet,
	)

	if apierrs.IsNotFound(err) {
		replicaSet := defineReplicaSet(peer, r.EtcdRepository, log)
		log.V(1).Info("Replica set does not exist, creating",
			"replica-set", replicaSet.Name)
		if err := r.Create(ctx, &replicaSet); err != nil {
			log.Error(err, "unable to create ReplicaSet for EtcdPeer", "replica-set", replicaSet)
			return result, err
		}
		return result, nil
	}

	// Check for some other error from the previous `r.Get`
	if err != nil {
		log.Error(err, "unable to query for replica sets")
		return result, err
	}

	log.V(2).Info("Replica set already exists", "replica-set", existingReplicaSet.Name)

	return result, nil
}

type pvcMapper struct{}

var _ handler.Mapper = &pvcMapper{}

// Map looks up the peer name label from the PVC and generates a reconcile
// request for *that* name in the namespace of the pvc.
// This mapper ensures that we only wake up the Reconcile function for changes
// to PVCs related to EtcdPeer resources.
// PVCs are deliberately not owned by the peer, to ensure that they are not
// garbage collected along with the peer.
// So we can't use OwnerReference handler here.
func (m *pvcMapper) Map(o handler.MapObject) []reconcile.Request {
	requests := []reconcile.Request{}
	labels := o.Meta.GetLabels()
	if peerName, found := labels[peerLabel]; found {
		requests = append(
			requests,
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      peerName,
					Namespace: o.Meta.GetNamespace(),
				},
			},
		)
	}
	return requests
}

func (r *EtcdPeerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdPeer{}).
		// Watch for changes to ReplicaSet resources that an EtcdPeer owns.
		Owns(&appsv1.ReplicaSet{}).
		// We can use a simple EnqueueRequestForObject handler here as the PVC
		// has the same name as the EtcdPeer resource that needs to be enqueued
		Watches(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: &pvcMapper{},
		}).
		Complete(r)
}
