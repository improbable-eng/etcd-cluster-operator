package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/spf13/pflag"
	flag "github.com/spf13/pflag"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/snapshot"
	"google.golang.org/grpc"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	pb "github.com/improbable-eng/etcd-cluster-operator/api/proxy/v1"
	"github.com/improbable-eng/etcd-cluster-operator/version"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

// loggedError logs an error locally and returns the error decorated with the
// log message so that it can be returned to the protobuf client.
func loggedError(log logr.Logger, err error, message string) error {
	log.Error(err, message)
	return fmt.Errorf("%s: %s", message, err)
}

func main() {
	proxyURL := pflag.String("proxy-url",
		"",
		"URL of the proxy server to use to upload the backup to remote storage.")

	backupTempDir := flag.String("backup-tmp-dir",
		os.TempDir(),
		"The directory to temporarily place backups before they are uploaded to their destination.")

	backupURL := pflag.String("backup-url",
		"",
		"URL for the backup.")

	etcdURL := pflag.String("etcd-url",
		"",
		"URL for etcd.")

	etcdDialTimeoutSeconds := pflag.Int64("etcd-dial-timeout-seconds",
		5,
		"Timeout, in seconds, for dialing the Etcd API.")

	timeoutSeconds := pflag.Int64("timeout-seconds",
		60,
		"Timeout, in seconds, of the whole restore operation.")

	printVersion := pflag.Bool("version",
		false,
		"Print version information and exit")

	flag.Parse()

	if *printVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}
	zapLogger := zap.NewRaw(zap.UseDevMode(true))
	ctrl.SetLogger(zapr.NewLogger(zapLogger))
	setupLog.Info(
		"Starting backup-agent",
		"version", version.Version,
		"proxy-url", *proxyURL,
		"backup-temp-dir", *backupTempDir,
		"backup-url", *backupURL,
		"etcd-url", *etcdURL,
		"etcd-dial-timeout-seconds", *etcdDialTimeoutSeconds,
		"timeout-seconds", *timeoutSeconds,
	)

	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*time.Duration(*timeoutSeconds))
	defer ctxCancel()

	log := ctrl.Log.WithName("backup-agent")

	log.Info("Connecting to Etcd and getting snapshot")
	localPath := filepath.Join(*backupTempDir, "snapshot.db")
	etcdClient := snapshot.NewV3(zapLogger.Named("etcd-client"))
	err := etcdClient.Save(
		ctx,
		clientv3.Config{
			Endpoints:   []string{*etcdURL},
			DialTimeout: time.Second * time.Duration(*etcdDialTimeoutSeconds),
		},
		localPath,
	)
	if err != nil {
		panic(loggedError(log, err, "failed to get etcd snapshot"))
	}

	log.Info("Opening snapshot file")
	sourceFile, err := os.Open(localPath)
	if err != nil {
		panic(loggedError(log, err, "failed to open snapshot file"))
	}
	defer sourceFile.Close()

	log.Info("Reading snapshot file")
	sourceBytes, err := ioutil.ReadAll(sourceFile)
	if err != nil {
		panic(loggedError(log, err, "failed to read snapshot file"))
	}

	log.Info("Dialing proxy")
	conn, err := grpc.Dial(*proxyURL, grpc.WithInsecure())
	if err != nil {
		panic(loggedError(log, err, "failed to dial proxy"))
	}
	client := pb.NewProxyServiceClient(conn)

	log.Info("Uploading snapshot")
	_, err = client.Upload(ctx, &pb.UploadRequest{
		BackupUrl: *backupURL,
		Backup:    sourceBytes,
	})
	if err != nil {
		panic(loggedError(log, err, "failed to upload backup"))
	}
	log.Info("Backup complete")
}
