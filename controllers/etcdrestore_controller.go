package controllers

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/internal/reconcilerevent"
)

// EtcdRestoreReconciler reconciles a EtcdRestore object
type EtcdRestoreReconciler struct {
	client.Client
	Log             logr.Logger
	Recorder        record.EventRecorder
	RestorePodImage string
	ProxyURL        string
}

// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdrestores,verbs=get;list;watch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdclusters,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=etcd.improbable.io,resources=etcdrestores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=persistantvolumeclaims/status,verbs=get;watch;update;patch;create
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;watch;list;create

func name(o metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      o.GetName(),
	}
}

const (
	restoreContainerName      = "etcd-restore"
	restoredFromLabel         = "etcd.improbable.io/restored-from"
	restorePodLabel           = "etcd.improbable.io/restore-pod"
	pvcCreatedEventReason     = "PVCCreated"
	pvcInvalidEventReason     = "PVCInvalid"
	podCreatedEventReason     = "PodCreated"
	podInvalidEventReason     = "PodInvalid"
	podFailedEventReason      = "PodFailed"
	clusterCreatedEventReason = "ClusterCreated"
)

func markPVC(restore etcdv1alpha1.EtcdRestore, pvc *corev1.PersistentVolumeClaim) {
	if pvc.Labels == nil {
		pvc.Labels = make(map[string]string)
	}
	pvc.Labels[restoredFromLabel] = restore.Name
}

func IsOurPVC(restore etcdv1alpha1.EtcdRestore, pvc corev1.PersistentVolumeClaim) bool {
	return pvc.Labels[restoredFromLabel] == restore.Name
}

func markPod(restore etcdv1alpha1.EtcdRestore, pod *corev1.Pod) {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[restoredFromLabel] = restore.Name
	pod.Labels[restorePodLabel] = "true"
}

func IsOurPod(restore etcdv1alpha1.EtcdRestore, pod corev1.Pod) bool {
	return pod.Labels[restoredFromLabel] == restore.Name &&
		pod.Labels[restorePodLabel] == "true"
}

func markCluster(restore etcdv1alpha1.EtcdRestore, cluster *etcdv1alpha1.EtcdCluster) {
	if cluster.Labels == nil {
		cluster.Labels = make(map[string]string)
	}
	cluster.Labels[restoredFromLabel] = restore.Name
}

func (r *EtcdRestoreReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log := r.Log.WithValues("etcdrestore", req.NamespacedName)

	log.Info("Begin restore reconcile")

	var restore etcdv1alpha1.EtcdRestore
	if err := r.Get(ctx, req.NamespacedName, &restore); err != nil {
		// Can't find the resource. We could have just been deleted? If so, no need to do anything as the owner
		// references will clean everything up with a cascading delete.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	} else {
		log.Info("Found restore object in Kubernetes, continuing")
		// Found the resource, continue with reconciliation.
	}

	if restore.Status.Phase == etcdv1alpha1.EtcdRestorePhaseCompleted ||
		restore.Status.Phase == etcdv1alpha1.EtcdRestorePhaseFailed {
		// Do nothing here. We're already finished.
		log.Info("Phase is set to an end state. Taking no further action", "phase", restore.Status.Phase)
		return ctrl.Result{}, nil
	} else if restore.Status.Phase == etcdv1alpha1.EtcdRestorePhasePending {
		// In any other state continue with the reconciliation. In particular we don't *read* the other possible states
		// as once we know that we need to reconcile at all we will use the observed sate of the cluster and not our own
		// status field.
		log.Info("Phase is not set to an end state. Continuing reconciliation.", "phase", restore.Status.Phase)
	} else {
		log.Info("Restore has no phase, setting to pending")
		restore.Status.Phase = etcdv1alpha1.EtcdRestorePhasePending
		err := r.Client.Status().Update(ctx, &restore)
		return ctrl.Result{}, err
	}

	// Simulate the cluster, peer, and PVC we'll create. We won't ever *create* any of these other than the PVC, but
	// we need them to generate the PVC using the same code-path that the main operator does. Yes, in theory if you
	// upgraded the operator part-way through a restore things could get super fun if the way of making these changed.
	expectedCluster := clusterForRestore(restore)
	expectedCluster.Default()

	expectedPeerNames := expectedPeerNamesForCluster(expectedCluster)
	expectedPeers := make([]etcdv1alpha1.EtcdPeer, len(expectedPeerNames))
	for i, peerName := range expectedPeerNames {
		expectedPeers[i] = *peerForCluster(expectedCluster, peerName)
		expectedPeers[i].Default()
		configurePeerBootstrap(&expectedPeers[i], expectedCluster)
	}
	expectedPVCs := make([]corev1.PersistentVolumeClaim, len(expectedPeers))
	for i, peer := range expectedPeers {
		expectedPVCs[i] = *pvcForPeer(&peer)
		markPVC(restore, &expectedPVCs[i])
	}

	// Step 1. Create the PVCs.
	for _, expectedPVC := range expectedPVCs {
		var pvc corev1.PersistentVolumeClaim
		err := r.Get(ctx, name(&expectedPVC), &pvc)

		if apierrors.IsNotFound(err) {
			// No PVC. Make it!
			log.Info("Creating PVC", "pvc-name", name(&expectedPVC))
			err := r.Create(ctx, &expectedPVC)
			if err != nil {
				return ctrl.Result{}, err
			}
			r.Recorder.Eventf(&restore,
				reconcilerevent.K8sEventTypeNormal,
				pvcCreatedEventReason,
				"Created PVC '%s/%s'",
				pvc.Namespace,
				pvc.Name)
			return ctrl.Result{}, nil
		} else if err != nil {
			// There was some other, non-notfound error. Exit as we can't handle this case.
			log.Info("Encountered error while finding PVC")
			return ctrl.Result{}, err
		}
		log.Info("PVC already exists. continuing", "pvc-name", name(&pvc))
		// The PVC is there already. Continue.

		// Check to make sure we're expecting to restore into this PVC
		if !IsOurPVC(restore, pvc) {
			// It's unsafe to take any further action. We don't control the PVC.
			log.Info("PVC is not marked as our PVC. Failing", "pvc-name", name(&pvc))
			// Construct error reason string
			var reason string
			if restoredFrom, found := pvc.Labels[restoredFromLabel]; found {
				reason = fmt.Sprintf("Label %s indicates a different restore %s made this PVC.",
					restoredFromLabel,
					restoredFrom)
			} else {
				reason = fmt.Sprintf("Label %s not set, indicating this is a pre-existing PVC.",
					restoredFromLabel)
			}
			r.Recorder.Eventf(&restore,
				reconcilerevent.K8sEventTypeWarning,
				pvcInvalidEventReason,
				"Found a PVC '%s/%s' as expected, but it wasn't labeled as ours: %s",
				pvc.Namespace,
				pvc.Name,
				reason)
			restore.Status.Phase = etcdv1alpha1.EtcdRestorePhaseFailed
			err := r.Client.Status().Update(ctx, &restore)
			return ctrl.Result{}, err
		}
		log.Info("PVC correctly marked as ours", "pvc-name", name(&pvc))
		// Great, this PVC is marked as *our* restore (and not any random PVC, or one for an existing cluster, or
		// a conflicting restore).
	}

	// So the peer restore is like a distributed fork/join. We need to launch n restore pods, where n is the number of
	// PVCs. Then wait for all of them to be complete. If any fail we abort.

	log.Info("Searching for restore pods")
	restorePods := make([]*corev1.Pod, len(expectedPeerNames))
	// Go find the restore pods for each peer
	for i, peer := range expectedPeers {
		log.Info("Searching for restore pod for peer", "peer-name", peer.Name)
		pod := corev1.Pod{}
		err := r.Get(ctx, restorePodNamespacedName(peer), &pod)
		if client.IgnoreNotFound(err) != nil {
			// This was a non-notfound error. Fail!
			return ctrl.Result{}, err
		} else if err == nil {
			restorePods[i] = &pod
		}
	}

	log.Info("Finished querying for restore pods")
	// Verify each pod we found is actually ours
	for _, restorePod := range restorePods {
		if restorePod != nil && !IsOurPod(restore, *restorePod) {
			// Construct error reason string
			var reason string
			if restoredFrom, found := restorePod.Labels[restoredFromLabel]; found {
				reason = fmt.Sprintf("Label %s indicates a different restore %s made this Pod.",
					restoredFromLabel,
					restoredFrom)
			} else {
				reason = fmt.Sprintf("Label %s not set, indicating this is a pre-existing Pod.",
					restoredFromLabel)
			}
			r.Recorder.Eventf(&restore,
				reconcilerevent.K8sEventTypeWarning,
				podInvalidEventReason,
				"Found a Pod '%s/%s' as expected, but it wasn't labeled as ours: %s",
				restorePod.Namespace,
				restorePod.Name,
				reason)
			log.Info("Restore Pod isn't ours", "pod-name", name(restorePod))
			restore.Status.Phase = etcdv1alpha1.EtcdRestorePhaseFailed
			err := r.Client.Status().Update(ctx, &restore)
			return ctrl.Result{}, err
		}
	}

	log.Info("Verified all existing restore pods are ours")
	// Launch any that are not found
	for i, restorePod := range restorePods {
		if restorePod == nil {
			// We couldn't find this restore pod. Create it.
			log.Info("Defining restore pod", "pod-index", i)
			toCreatePod := r.podForRestore(restore, expectedPeers[i], expectedPVCs[i].Name)
			log.Info("Launching restore pod", "pod-name", name(toCreatePod))
			// No Pod. Launch it! We then exit. Next time we'll make the next one.
			err := r.Create(ctx, toCreatePod)
			if err != nil {
				return ctrl.Result{}, err
			}
			r.Recorder.Eventf(&restore,
				reconcilerevent.K8sEventTypeNormal,
				podCreatedEventReason,
				"Launched Pod '%s/%s' to restore data to PVC '%s/%s'",
				toCreatePod.Namespace,
				toCreatePod.Name,
				expectedPVCs[i].Namespace,
				expectedPVCs[i].Name)
			return ctrl.Result{}, nil
		}
	}
	log.Info("All restore pods are launched")
	// All restore pods exist and our ours, so have they finished? This would be the join part of the "funky fork/join".
	anyFailed := false
	allSuccess := true
	for _, restorePod := range restorePods {
		phase := restorePod.Status.Phase
		if phase == corev1.PodSucceeded {
			// Success is what we want. Continue with reconciliation.
			log.Info("Restore Pod has succeeded", "pod-name", name(restorePod))
		} else if phase == corev1.PodFailed || phase == corev1.PodUnknown {
			// Pod has failed, so we fail
			log.Info("Restore Pod was not successful", "pod-name", name(restorePod), "phase", phase)
			allSuccess = false
			anyFailed = true
		} else {
			// This covers the "Pending" and "Running" phases. Do nothing and wait for the Pod to finish.
			log.Info("Restore Pod still running.", "pod-name", name(restorePod), "phase", phase)
			allSuccess = false
		}
	}

	if anyFailed {
		log.Info("One or more restore pods were not successful. Marking the restore as failed.")
		restore.Status.Phase = etcdv1alpha1.EtcdRestorePhaseFailed
		err := r.Client.Status().Update(ctx, &restore)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(&restore,
			reconcilerevent.K8sEventTypeWarning,
			podFailedEventReason,
			"One or more Pods failed, please inspect the Pod logs for further details.")
		return ctrl.Result{}, err
	}
	if !allSuccess {
		log.Info("One or more restore pods are still executing, and none have failed. Waiting...")
		// Hrm. Not all finished yet. Exit and do nothing.
		return ctrl.Result{}, nil
	}

	// Create the cluster
	log.Info("All restore pods are complete, creating etcd cluster")
	cluster := etcdv1alpha1.EtcdCluster{}
	err := r.Get(ctx, name(expectedCluster), &cluster)
	if apierrors.IsNotFound(err) {
		// No Cluster. Create it
		log.Info("Creating new cluster", "cluster-name", name(expectedCluster))
		err := r.Create(ctx, expectedCluster)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Eventf(&restore,
			reconcilerevent.K8sEventTypeNormal,
			clusterCreatedEventReason,
			"Created EtcdCluster '%s/%s'.",
			expectedCluster.Namespace,
			expectedCluster.Name)
		return ctrl.Result{}, err
	} else if err != nil {
		log.Info("Error creating cluster", "cluster-name", name(expectedCluster))
		return ctrl.Result{}, err
	}

	// This is the end. Our cluster exists.
	log.Info("Verified cluster exists, exiting.")
	restore.Status.Phase = etcdv1alpha1.EtcdRestorePhaseCompleted
	err = r.Client.Status().Update(ctx, &restore)
	return ctrl.Result{}, err
}

func restorePodName(peer etcdv1alpha1.EtcdPeer) string {
	return fmt.Sprintf("%s-restore", peer.GetName())
}

func restorePodNamespacedName(peer etcdv1alpha1.EtcdPeer) types.NamespacedName {
	return types.NamespacedName{
		Namespace: peer.GetNamespace(),
		Name:      restorePodName(peer),
	}
}

func stripPortFromURL(advertiseURL *url.URL) string {
	return strings.Split(advertiseURL.Host, ":")[0]
}

func (r *EtcdRestoreReconciler) podForRestore(restore etcdv1alpha1.EtcdRestore, peer etcdv1alpha1.EtcdPeer, pvcName string) *corev1.Pod {
	const snapshotDir = "/tmp/snapshot"
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            restorePodName(peer),
			Namespace:       restore.Namespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(&restore, etcdv1alpha1.GroupVersion.WithKind("EtcdRestore"))},
		},
		Spec: corev1.PodSpec{
			HostAliases: []corev1.HostAlias{
				{
					// We've got a problematic Catch-22 here. We need to set the peer advertise URL to be the real URL
					// that the eventual peer will use. We know that URL at this point (and provide it as an environment
					// variable later on).
					//
					// However without the Kubernetes service to provide DNS and the Hostnames set on the pods the URL
					// can't actually be resolved. Unfortunately, the etcd API will try to resolve the advertise URL to
					// check it's real.
					//
					// So, we need to fake it so it resolves to something. It doesn't need to have anything on the other
					// end. So long as an IP address comes back the etcd API is satisfied.
					IP: net.IPv4(127, 0, 0, 1).String(),
					Hostnames: []string{
						stripPortFromURL(advertiseURL(peer, etcdPeerPort)),
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "etcd-data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
				{
					// This is scratch space to download the snapshot file into
					Name: "snapshot",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: nil,
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:      restoreContainerName,
					Image:     r.RestorePodImage,
					Args:      []string{},
					Resources: *restore.Spec.ClusterTemplate.Spec.PodTemplate.Resources,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "etcd-data",
							ReadOnly:  false,
							MountPath: EtcdDataMountPath,
						},
						{
							Name:      "snapshot",
							ReadOnly:  false,
							MountPath: snapshotDir,
						},
					},
					// TODO: Add resource requests and affinity rules which
					// match the eventual cluster peer pod.
					// See https://github.com/improbable-eng/etcd-cluster-operator/issues/172
				},
			},
			// If we fail, we fail
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// Create a helper to append flags
	stringFlag := func(flag string, value string) {
		// We know there's only one container and it's the first in the list
		pod.Spec.Containers[0].Args = append(pod.Spec.Containers[0].Args, fmt.Sprintf("--%s=%s", flag, value))
	}
	stringFlag("etcd-peer-name", peer.Name)
	stringFlag("etcd-cluster-name", restore.Spec.ClusterTemplate.ClusterName)
	stringFlag("etcd-initial-cluster", staticBootstrapInitialCluster(*peer.Spec.Bootstrap.Static, etcdScheme(peer.Spec.TLS)))
	stringFlag("etcd-peer-advertise-url", advertiseURL(peer, etcdPeerPort).String())
	stringFlag("etcd-data-dir", EtcdDataMountPath)
	stringFlag("snapshot-dir", snapshotDir)
	stringFlag("backup-url", restore.Spec.Source.ObjectURL)
	stringFlag("proxy-url", r.ProxyURL)

	markPod(restore, &pod)
	return &pod
}

func clusterForRestore(restore etcdv1alpha1.EtcdRestore) *etcdv1alpha1.EtcdCluster {
	cluster := &etcdv1alpha1.EtcdCluster{
		ObjectMeta: v1.ObjectMeta{
			Name:      restore.Spec.ClusterTemplate.ClusterName,
			Namespace: restore.Namespace,
		},
		Spec: restore.Spec.ClusterTemplate.Spec,
	}
	// Slap a label on it. This is pretty informational. Just so a user can tell where this thing came from later on.
	markCluster(restore, cluster)
	return cluster
}

type restoredFromMapper struct{}

var _ handler.Mapper = &restoredFromMapper{}

// Map looks up the peer name label from the PVC and generates a reconcile
// request for *that* name in the namespace of the pvc.
// This mapper ensures that we only wake up the Reconcile function for changes
// to PVCs related to EtcdPeer resources.
// PVCs are deliberately not owned by the peer, to ensure that they are not
// garbage collected along with the peer.
// So we can't use OwnerReference handler here.
func (m *restoredFromMapper) Map(o handler.MapObject) []reconcile.Request {
	var requests []reconcile.Request
	labels := o.Meta.GetLabels()
	if restoreName, found := labels[restoredFromLabel]; found {
		requests = append(
			requests,
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      restoreName,
					Namespace: o.Meta.GetNamespace(),
				},
			},
		)
	}
	return requests
}

func (r *EtcdRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&etcdv1alpha1.EtcdRestore{}).
		// Watch for changes to Pod resources that an EtcdRestore owns.
		Owns(&corev1.Pod{}).
		// Watch for changes to PVCs with a 'restored-from' label.
		Watches(&source.Kind{Type: &corev1.PersistentVolumeClaim{}}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: &restoredFromMapper{},
		}).
		Watches(&source.Kind{Type: &etcdv1alpha1.EtcdCluster{}}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: &restoredFromMapper{},
		}).
		Complete(r)
}
