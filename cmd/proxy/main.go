package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"

	humanize "github.com/dustin/go-humanize"
	"github.com/go-logr/logr"
	flag "github.com/spf13/pflag"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/gcsblob"
	_ "gocloud.dev/blob/s3blob"
	"google.golang.org/grpc"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	pb "github.com/improbable-eng/etcd-cluster-operator/api/proxy/v1"
	"github.com/improbable-eng/etcd-cluster-operator/version"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

type proxyServer struct {
	pb.UnimplementedProxyServiceServer
	log logr.Logger
}

// Turn a full object URL like `gs://my-bucket/my-dir/my-obj.db` into a bucket URL (`gs://my-bucket`) and an object path
// (`/my-dir/my-obj.db`).
func parseBackupURL(backupUrl string) (string, string, error) {
	u, err := url.Parse(backupUrl)
	if err != nil {
		return "", "", err
	}
	path := u.Path
	u.Path = ""
	return u.String(), path, nil
}

// loggedError logs an error locally and returns the error decorated with the
// log message so that it can be returned to the protobuf client.
func loggedError(log logr.Logger, err error, message string) error {
	log.Error(err, message)
	return fmt.Errorf("%s: %s", message, err)
}

func (ps *proxyServer) Upload(ctx context.Context, req *pb.UploadRequest) (_ *pb.UploadResponse, reterr error) {
	backupURL := req.GetBackupUrl()
	log := ps.log.WithName("upload").WithValues("req-backup-url", backupURL)
	log.Info("Started")
	defer log.Info("Finished")

	bucketName, objectPath, err := parseBackupURL(backupURL)
	if err != nil {
		return nil, loggedError(log, err, "failed to parse backup URL")
	}
	bucket, err := blob.OpenBucket(ctx, bucketName)
	if err != nil {
		return nil, loggedError(log, err, "failed to open bucket")
	}

	blobWriter, err := bucket.NewWriter(ctx, objectPath, nil)
	if err != nil {
		return nil, loggedError(log, err, "failed to create blob writer")
	}
	defer func() {
		if err := blobWriter.Close(); err != nil {
			err = loggedError(log, err, "failed to close blob writer")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}
	}()
	// Here we have the entire contents of the backup into memory. In theory
	// these could be quite big (multiple gigabytes). So we're actually taking a
	// risk that the backup could be *too big* for our available memory.
	written, err := blobWriter.Write(req.Backup)
	if err != nil {
		return nil, loggedError(log, err, "failed to write blob")
	}

	log.V(2).Info("Returning response", "backup-size", humanize.Bytes(uint64(written)))

	return &pb.UploadResponse{}, nil
}

func (ps *proxyServer) Download(ctx context.Context, req *pb.DownloadRequest) (*pb.DownloadResponse, error) {
	log := ps.log.WithName("download").WithValues("req-backup-url", req.GetBackupUrl())

	bucketName, objectPath, err := parseBackupURL(req.BackupUrl)
	if err != nil {
		return nil, loggedError(log, err, "failed to parse backup URL")
	}
	bucket, err := blob.OpenBucket(ctx, bucketName)
	if err != nil {
		return nil, loggedError(log, err, "failed to open bucket")
	}

	blobReader, err := bucket.NewReader(ctx, objectPath, nil)
	if err != nil {
		return nil, loggedError(log, err, "failed to create blob reader")
	}
	defer func() {
		if err := blobReader.Close(); err != nil {
			log.Error(err, "failed to close blob reader")
		}
	}()
	// Here we read the entire contents of the backup into memory. In theory these could be quite big (multiple
	// gigabytes). So we're actually taking a risk that the backup could be *too big* for our available memory.
	backup, err := ioutil.ReadAll(blobReader)
	if err != nil {
		return nil, loggedError(log, err, "failed to read blob")
	}

	log.V(2).Info("Returning response", "backup-size", humanize.Bytes(uint64(len(backup))))
	return &pb.DownloadResponse{Backup: backup}, nil
}

func main() {
	printVersion := flag.Bool("version", false, "Print the version to stdout and exit")
	apiPort := flag.Int("api-port", 8080, "Port to serve the API on")
	flag.Parse()

	if *printVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	setupLog.Info("Starting proxy", "version", version.Version)

	// Launch gRPC server
	grpcAddress := fmt.Sprintf(":%d", *apiPort)
	setupLog.Info("Listening", "grpc-address", grpcAddress)
	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		setupLog.Error(err, "Failed to listen")
		os.Exit(1)
	}

	srv := grpc.NewServer()
	pb.RegisterProxyServiceServer(srv, &proxyServer{log: ctrl.Log.WithName("proxy-server")})
	if err := srv.Serve(listener); err != nil {
		setupLog.Error(err, "Failed to serve")
		os.Exit(1)
	}
}
