package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/otiai10/copy"
	"github.com/spf13/pflag"
	"go.etcd.io/etcd/clientv3/snapshot"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
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

	etcdPeerName := pflag.String("etcd-peer-name",
		"",
		"Name of this peer, must match the name of the eventual peer")

	etcdClusterName := pflag.String("etcd-cluster-name",
		"",
		"Name of this cluster, must match the name of the eventual cluster")

	etcdInitialCluster := pflag.String("etcd-initial-cluster",
		"",
		"Comma separated list of the peer advertise URLs of the complete eventual cluster, including our own.")

	etcdAdvertiseURL := pflag.String("etcd-peer-advertise-url",
		"",
		"The peer advertise URL of *this* peer, must match the one for the eventual cluster")

	etcdDataDir := pflag.String("etcd-data-dir",
		"/var/etcd",
		"Location of the etcd data directory to restore into.")

	snapshotDir := pflag.String("snapshot-dir",
		"/tmp/snapshot",
		"Location of a temporary directory to make the backup into")

	proxyURL := pflag.String("proxy-url",
		"",
		"URL of the proxy server to use to download the backup from remote storage.")

	backupURL := pflag.String("backup-url",
		"",
		"URL for the backup.")

	timeoutSeconds := pflag.Int64("timeout-seconds",
		300,
		"Timeout, in seconds, of the whole restore operation.")

	printVersion := pflag.Bool("version",
		false,
		"Print version information and exit")

	pflag.Parse()

	if *printVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	ctrl.SetLogger(zap.Logger(true))
	setupLog.Info(
		"Starting restore-agent",
		"version", version.Version,
		"etcd-peer-name", *etcdPeerName,
		"etcd-cluster-name", *etcdClusterName,
		"etcd-initial-cluster", *etcdInitialCluster,
		"etcd-advertise-url", *etcdAdvertiseURL,
		"etcd-data-directory", *etcdDataDir,
		"snapshot-directory", *snapshotDir,
		"proxy-url", *proxyURL,
		"backup-url", *backupURL,
		"timeout", *timeoutSeconds,
	)

	// Pull the object from cloud storage into the snapshot directory.
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*time.Duration(*timeoutSeconds))
	defer ctxCancel()

	log := ctrl.Log.WithName("restore-agent")

	log.Info("Dialing proxy")
	conn, err := grpc.Dial(*proxyURL, grpc.WithInsecure())
	if err != nil {
		panic(loggedError(log, err, "failed to dial proxy"))
	}

	c := pb.NewProxyServiceClient(conn)

	log.V(2).Info("Downloading")
	r, err := c.Download(ctx, &pb.DownloadRequest{
		// The inconsistent capitalisation of 'URL' is because of https://github.com/golang/protobuf/issues/156
		BackupUrl: *backupURL,
	})
	if err != nil {
		panic(loggedError(log, err, "failed to download"))
	}

	log.V(2).Info("Closing proxy connection")
	err = conn.Close()
	if err != nil {
		panic(loggedError(log, err, "failed to close proxy connection"))
	}

	log.V(2).Info("Download complete", "backup-size", len(r.Backup))

	snapshotFilePath := filepath.Join(*snapshotDir, "snapshot.db")
	log.V(2).Info("Saving object", "local-storage-location", snapshotFilePath)
	snapshotFile, err := os.Create(snapshotFilePath)
	if err != nil {
		panic(loggedError(log, err, "failed to create snapshot file"))
	}
	snapshotFileWriter := bufio.NewWriter(snapshotFile)
	_, err = io.Copy(snapshotFileWriter, bytes.NewReader(r.Backup))
	if err != nil {
		panic(loggedError(log, err, "failed to copy backup to snapshot file"))
	}
	err = snapshotFileWriter.Flush()
	if err != nil {
		panic(loggedError(log, err, "failed to flush snapshot file"))
	}

	restoreDir := filepath.Join(*snapshotDir, "data-dir")

	restoreConfig := snapshot.RestoreConfig{
		SnapshotPath:        snapshotFilePath,
		Name:                *etcdPeerName,
		OutputDataDir:       restoreDir,
		OutputWALDir:        filepath.Join(restoreDir, "member", "wal"),
		PeerURLs:            []string{*etcdAdvertiseURL},
		InitialCluster:      *etcdInitialCluster,
		InitialClusterToken: *etcdClusterName,
		SkipHashCheck:       false,
	}

	client := snapshot.NewV3(nil)
	log.V(2).Info("Executing restore")
	err = client.Restore(restoreConfig)
	if err != nil {
		panic(loggedError(log, err, "failed to restore"))
	}
	log.V(2).Info("Copying restored data directory into PV", "restore-dir", restoreDir, "etcd-data-dir", *etcdDataDir)
	err = copy.Copy(restoreDir, *etcdDataDir)
	if err != nil {
		panic(loggedError(log, err, "failed to copy restored data into PV"))
	}
	log.V(2).Info("Restore complete")
}
