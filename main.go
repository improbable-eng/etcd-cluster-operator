package main

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"os"
	"strings"

	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	etcdv1alpha1 "github.com/improbable-eng/etcd-cluster-operator/api/v1alpha1"
	"github.com/improbable-eng/etcd-cluster-operator/controllers"
	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
	"github.com/improbable-eng/etcd-cluster-operator/version"
	"github.com/improbable-eng/etcd-cluster-operator/webhooks"
	"github.com/robfig/cron/v3"
	// +kubebuilder:scaffold:imports
)

var (
	scheme                = runtime.NewScheme()
	setupLog              = ctrl.Log.WithName("setup")
	defaultEtcdRepository = "quay.io/coreos/etcd"
	// These are replaced as part of the build so that the images match the name
	// prefix and version of the controller image. See Dockerfile.
	defaultBackupAgentImage  = "REPLACE_ME"
	defaultRestoreAgentImage = "REPLACE_ME"
	leaderRetryDuration      = 5 * time.Second
)

const (
	defaultProxyPort = 80
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = etcdv1alpha1.AddToScheme(scheme)
	_ = monitorv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		enableLeaderElection      bool
		leaderElectionID          string
		leaderElectionCMNamespace string
		metricsAddr               string
		printVersion              bool
		proxyURL                  string
		etcdRepository            string
		restoreAgentImage         string
		backupAgentImage          string
		defragThreshold           uint
		leaderRenewSeconds        uint
	)

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", true,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionID, "leader-election-id", "etcd-cluster-operator-controller-leader-election-helper",
		"The name of the configmap that leader election will use for holding the leader lock.")
	flag.StringVar(&leaderElectionCMNamespace, "leader-election-cm-namespace", "storageos", "The namespace of the config map that is used for leader election")
	flag.StringVar(&etcdRepository, "etcd-repository", defaultEtcdRepository, "The Docker repository to use for the etcd image")
	flag.StringVar(&backupAgentImage, "backup-agent-image", defaultBackupAgentImage, "The Docker image for the backup-agent")
	flag.StringVar(&restoreAgentImage, "restore-agent-image", defaultRestoreAgentImage, "The Docker image to use to perform a restore")
	flag.StringVar(&proxyURL, "proxy-url", "", "The URL of the upload/download proxy")
	flag.BoolVar(&printVersion, "version", false,
		"Print version to stdout and exit")
	flag.UintVar(&defragThreshold, "defrag-threshold", 80, "The percentage of used space at which the operator will defrag an etcd member")
	flag.UintVar(&leaderRenewSeconds, "leader-renew-seconds", 10, "Leader renewal frequency - for leader election")
	flag.Parse()

	renewDeadline := time.Duration(leaderRenewSeconds) * time.Second
	leaseDuration := time.Duration(int(1.2*float64(leaderRenewSeconds))) * time.Second

	if printVersion {
		fmt.Println(version.Version)
		return
	}
	// TODO: Allow users to configure JSON logging.
	// See https://github.com/improbable-eng/etcd-cluster-operator/issues/171
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	setupLog.Info(
		"Starting manager",
		"version", version.Version,
		"etcd-repository", etcdRepository,
		"backup-agent-image", backupAgentImage,
		"restore-agent-image", restoreAgentImage,
		"proxy-url", proxyURL,
	)

	if !strings.Contains(proxyURL, ":") {
		// gRPC needs a port, and this address doesn't seem to have one.
		proxyURL = fmt.Sprintf("%s:%d", proxyURL, defaultProxyPort)
		setupLog.Info("Defaulting port on configured Proxy URL",
			"default-proxy-port", defaultProxyPort,
			"proxy-url", proxyURL)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                        scheme,
		MetricsBindAddress:            metricsAddr,
		LeaderElection:                enableLeaderElection,
		LeaderElectionID:              leaderElectionID,
		LeaderElectionNamespace:       leaderElectionCMNamespace,
		LeaderElectionReleaseOnCancel: true,
		LeaderElectionResourceLock:    resourcelock.LeasesResourceLock,
		RenewDeadline:                 &renewDeadline,
		LeaseDuration:                 &leaseDuration,
		RetryPeriod:                   &leaderRetryDuration,
		Port:                          9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	cronHandler := cron.New(cron.WithChain(
		cron.SkipIfStillRunning(ctrl.Log),
	))

	if err = (&controllers.EtcdPeerReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("EtcdPeer"),
		Recorder: mgr.GetEventRecorderFor("etcdpeer-reconciler"),

		Etcd:           &etcd.ClientEtcdAPIBuilder{},
		EtcdRepository: etcdRepository,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdPeer")
		os.Exit(1)
	}
	if err = (&controllers.EtcdClusterReconciler{
		Client:          mgr.GetClient(),
		Log:             ctrl.Log.WithName("controllers").WithName("EtcdCluster"),
		Recorder:        mgr.GetEventRecorderFor("etcdcluster-reconciler"),
		Etcd:            &etcd.ClientEtcdAPIBuilder{},
		CronHandler:     cronHandler,
		Schedules:       controllers.NewScheduleMap(),
		DefragThreshold: defragThreshold,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "EtcdCluster")
		os.Exit(1)
	}
	if err = (&controllers.EtcdBackupReconciler{
		Client:           mgr.GetClient(),
		Log:              ctrl.Log.WithName("controllers").WithName("EtcdBackup"),
		Scheme:           mgr.GetScheme(),
		BackupAgentImage: backupAgentImage,
		ProxyURL:         proxyURL,
		Recorder:         mgr.GetEventRecorderFor("etcdbackup-reconciler"),
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
		Recorder:        mgr.GetEventRecorderFor("etcdrestore-reconciler"),
		RestorePodImage: restoreAgentImage,
		ProxyURL:        proxyURL,
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

	if os.Getenv("ENABLE_PPROF") != "" {
		setupLog.Info("Running profiling webserver")
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/debug/pprof", pprof.Index)
			err := http.ListenAndServe(":7777", mux)
			if err != nil {
				setupLog.Error(err, "pprof http error")
				os.Exit(1)
			}
		}()
	}

	setupLog.Info("Starting cron handler")
	cronHandler.Start()
	defer func() {
		setupLog.Info("Stopping cron handler")
		<-cronHandler.Stop().Done()
	}()
	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
