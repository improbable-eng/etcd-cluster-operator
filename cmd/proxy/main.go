package main

import (
	"fmt"
	"net"
	"os"

	flag "github.com/spf13/pflag"
	"google.golang.org/grpc"
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
}

func main() {
	printVersion := flag.Bool("version", false, "Print the version to stdout and exit")
	apiPort := flag.Int("api-port", 8080, "Port to serve the API on")
	flag.Parse()

	if *printVersion {
		fmt.Println(version.Version)
		os.Exit(0)
	}
	ctrl.SetLogger(zap.Logger(true))

	setupLog.Info("Starting etcd-cluster-controller upload/download Proxy service", "version", version.Version)

	// Launch gRPC server
	grpcAddress := fmt.Sprintf(":%d", *apiPort)
	setupLog.Info("Listening", "grpc-address", grpcAddress)
	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		setupLog.Error(err, "Failed to listen")
		os.Exit(1)
	}

	srv := grpc.NewServer()
	pb.RegisterProxyServiceServer(srv, &proxyServer{})
	if err := srv.Serve(listener); err != nil {
		setupLog.Error(err, "Failed to serve")
		os.Exit(1)
	}
}
