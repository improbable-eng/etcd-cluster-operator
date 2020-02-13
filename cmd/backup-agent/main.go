package main

import (
	"fmt"

	flag "github.com/spf13/pflag"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/improbable-eng/etcd-cluster-operator/version"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func main() {
	var printVersion bool

	flag.BoolVar(&printVersion, "version", false, "Print the version to stdout and exit")
	flag.Parse()

	if printVersion {
		fmt.Println(version.Version)
		return
	}
	ctrl.SetLogger(zap.Logger(true))

	setupLog.Info("Starting backup-agent", "version", version.Version)
}
