package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/controllers"
	"github.com/improbable-eng/etcd-cluster-operator/version"
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
	var (
		printVersion  bool
		backupTempDir string
	)

	flag.BoolVar(&printVersion, "version", false, "Print the version to stdout and exit")
	flag.StringVar(&backupTempDir, "backup-tmp-dir", os.TempDir(), "The directory to temporarily place backups before they are uploaded to their destination.")
	flag.Parse()

	if printVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}
	ctrl.SetLogger(zap.Logger(true))

	setupLog.Info("Starting backup-agent", "version", version.Version)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
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

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
