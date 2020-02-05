package main

import (
	"fmt"
	"log"
	"net"

	flag "github.com/spf13/pflag"
	"google.golang.org/grpc"

	pb "github.com/improbable-eng/etcd-cluster-operator/api/proxy"
)

type proxyServer struct {
	pb.UnimplementedProxyServer
}

func main() {
	// Setup defaults for expected configuration keys
	var apiPort = flag.Int("api-port", 8080, "Port to serve the API on.")
	flag.Parse()

	// Launch gRPC server
	grpcAddress := fmt.Sprintf(":%d", *apiPort)
	log.Printf("Using %q as listen address for proxy server", grpcAddress)
	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	log.Printf("Starting etcd-cluster-controller upload/download Proxy service")
	srv := grpc.NewServer()
	pb.RegisterProxyServer(srv, &proxyServer{})
	if err := srv.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
