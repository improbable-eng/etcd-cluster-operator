package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/otiai10/copy"
	"github.com/spf13/viper"
	"go.etcd.io/etcd/clientv3/snapshot"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
)

func main() {
	viper.SetEnvPrefix("restore")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("timeout-seconds", "300")

	etcdPeerName := viper.GetString("etcd-peer-name")
	fmt.Printf("Using etcd peer name %s\n", etcdPeerName)

	etcdClusterName := viper.GetString("etcd-cluster-name")
	fmt.Printf("Using etcd cluster name %s\n", etcdClusterName)

	etcdInitialCluster := viper.GetString("etcd-initial-cluster")
	fmt.Printf("Using etcd initial cluster %s\n", etcdInitialCluster)

	etcdAdvertiseURL := viper.GetString("etcd-advertise-url")
	fmt.Printf("Using advertise URL %s\n", etcdAdvertiseURL)

	etcdDataDir := viper.GetString("etcd-data-dir")
	fmt.Printf("Using etcd data directory %s\n", etcdDataDir)

	snapshotDir := viper.GetString("snapshot-dir")
	fmt.Printf("Using snapshot directory %s\n", snapshotDir)

	bucketURL := viper.GetString("bucket-url")
	fmt.Printf("Using Bucket URL %s\n", bucketURL)

	objectPath := viper.GetString("object-path")
	fmt.Printf("Using Object Path %s\n", objectPath)

	timeoutSeconds := viper.GetInt64("timeout-seconds")
	fmt.Printf("Using %d seconds timeout`n", timeoutSeconds)

	// Pull the object from cloud storage into the snapshot directory.
	fmt.Printf("Pulling object from %s/%s\n", bucketURL, objectPath)
	ctx, ctxCancel := context.WithTimeout(context.Background(), time.Second*time.Duration(timeoutSeconds))
	defer ctxCancel()

	bucket, err := blob.OpenBucket(ctx, bucketURL)
	if err != nil {
		panic(err)
	}
	blobReader, err := bucket.NewReader(ctx, objectPath, nil)
	if err != nil {
		panic(err)
	}
	snapshotFilePath := filepath.Join(snapshotDir, "snapshot.db")
	fmt.Printf("Saving Object to local storage location %s\n", snapshotFilePath)
	snapshotFile, err := os.Create(snapshotFilePath)
	if err != nil {
		panic(err)
	}
	snapshotFileWriter := bufio.NewWriter(snapshotFile)
	_, err = io.Copy(snapshotFileWriter, blobReader)
	if err != nil {
		panic(err)
	}
	err = snapshotFileWriter.Flush()
	if err != nil {
		panic(err)
	}
	err = bucket.Close()
	if err != nil {
		panic(err)
	}

	restoreDir := filepath.Join(snapshotDir, "data-dir")

	restoreConfig := snapshot.RestoreConfig{
		SnapshotPath:        snapshotFilePath,
		Name:                etcdPeerName,
		OutputDataDir:       restoreDir,
		OutputWALDir:        filepath.Join(restoreDir, "member", "wal"),
		PeerURLs:            []string{etcdAdvertiseURL},
		InitialCluster:      etcdInitialCluster,
		InitialClusterToken: etcdClusterName,
		SkipHashCheck:       false,
	}

	client := snapshot.NewV3(nil)
	fmt.Printf("Executing restore\n")
	err = client.Restore(restoreConfig)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Copying restored data directory %s into correct PV on path %s\n", restoreDir, etcdDataDir)
	err = copy.Copy(restoreDir, etcdDataDir)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Restore complete\n")
}
