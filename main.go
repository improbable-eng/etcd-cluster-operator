package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/controllers"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
	"github.com/improbable-eng/etcd-cluster-operator/webhooks"
	"github.com/robfig/cron/v3"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = etcdv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr, backupTempDir string
	var enableLeaderElection bool
	var restoreImageName string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&backupTempDir, "backup-tmp-dir", os.TempDir(), "The directory to temporarily place backups before they are uploaded to their destination.")
	flag.StringVar(&restoreImageName, "restore-image-name", "", "The Docker image to use to perform a restore")
	flag.Parse()

	ctrl.SetLogger(zap.Logger(true))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.EtcdPeerReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("EtcdPeer"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdPeer")
		os.Exit(1)
	}
	if err = (&controllers.EtcdClusterReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("EtcdCluster"),
		Recorder: mgr.GetEventRecorderFor("etcdcluster-reconciler"),
		Etcd:     &etcd.ClientEtcdAPI{},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdCluster")
		os.Exit(1)
	}
	if err = (&controllers.EtcdBackupReconciler{
		Client:  mgr.GetClient(),
		Log:     ctrl.Log.WithName("controllers").WithName("EtcdBackup"),
		TempDir: backupTempDir,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdBackup")
		os.Exit(1)
	}
	if err = (&controllers.EtcdBackupScheduleReconciler{
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("EtcdBackupSchedule"),
		CronHandler: cron.New(),
		Schedules:   controllers.NewScheduleMap(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdBackupSchedule")
		os.Exit(1)
	}
	if err = (&controllers.EtcdRestoreReconciler{
		Client:          mgr.GetClient(),
		Log:             ctrl.Log.WithName("controllers").WithName("EtcdRestore"),
		RestorePodImage: restoreImageName,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdRestore")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder
	if os.Getenv("DISABLE_WEBHOOKS") != "" {
		setupLog.Info("Skipping webhook set up.")
	} else {
		setupLog.Info("Setting up webhooks.")
		if err = (&webhooks.EtcdCluster{
			Log: ctrl.Log.WithName("webhooks").WithName("EtcdCluster"),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Unable to create webhook.", "webhook", "EtcdCluster")
			os.Exit(1)
		}
		if err = (&webhooks.EtcdPeer{
			Log: ctrl.Log.WithName("webhooks").WithName("EtcdPeer"),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Unable to create webhook.", "webhook", "EtcdPeer")
			os.Exit(1)
		}
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
