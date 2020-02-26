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

	"github.com/otiai10/copy"
	"github.com/spf13/pflag"
	"go.etcd.io/etcd/clientv3/snapshot"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
	"google.golang.org/grpc"

	pb "github.com/improbable-eng/etcd-cluster-operator/api/proxy/v1"
	"github.com/improbable-eng/etcd-cluster-operator/version"
)

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

	verbose := pflag.Bool("verbose", false, "Print out verbose information")

	pflag.Parse()

	if *verbose {
		fmt.Printf("Using etcd peer name: %s\n", *etcdPeerName)
		fmt.Printf("Using etcd cluster name: %s\n", *etcdClusterName)
		fmt.Printf("Using etcd initial cluster: %s\n", *etcdInitialCluster)
		fmt.Printf("Using etcd advertise URL: %s\n", *etcdAdvertiseURL)
		fmt.Printf("Using etcd data directory: %s\n", *etcdDataDir)
		fmt.Printf("Using snapshot directory: %s\n", *snapshotDir)
		fmt.Printf("Using proxy URL: %s\n", *proxyURL)
		fmt.Printf("Using backup URL: %s\n", *backupURL)
		fmt.Printf("Using timeout: %d seconds\n", *timeoutSeconds)
	}

	if *printVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}

	// Pull the object from cloud storage into the snapshot directory.
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*time.Duration(*timeoutSeconds))
	defer ctxCancel()

	fmt.Printf("Downloading backup file from Proxy %s\n", *backupURL)
	conn, err := grpc.Dial(*proxyURL, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}

	c := pb.NewProxyServiceClient(conn)
	r, err := c.Download(ctx, &pb.DownloadRequest{
		// The inconsistent capitalisation of 'URL' is because of https://github.com/golang/protobuf/issues/156
		BackupUrl: *backupURL,
	})
	if err != nil {
		panic(err)
	}

	err = conn.Close()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Have backup file of size %d", len(r.Backup))

	snapshotFilePath := filepath.Join(*snapshotDir, "snapshot.db")
	fmt.Printf("Saving Object to local storage location %s\n", snapshotFilePath)
	snapshotFile, err := os.Create(snapshotFilePath)
	if err != nil {
		panic(err)
	}
	snapshotFileWriter := bufio.NewWriter(snapshotFile)
	_, err = io.Copy(snapshotFileWriter, bytes.NewReader(r.Backup))
	if err != nil {
		panic(err)
	}
	err = snapshotFileWriter.Flush()
	if err != nil {
		panic(err)
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
	fmt.Printf("Executing restore\n")
	err = client.Restore(restoreConfig)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Copying restored data directory %s into correct PV on path %s\n", restoreDir, *etcdDataDir)
	err = copy.Copy(restoreDir, *etcdDataDir)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Restore complete\n")
}
