package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/spf13/viper"
	"google.golang.org/grpc"

	pb "github.com/improbable-eng/etcd-cluster-operator/api/proxy"
)

type proxyServer struct {
	pb.UnimplementedProxyServer
}

func (ps *proxyServer) Backup(ctx context.Context, request *pb.BackupRequest) (*pb.BackupReply, error) {
	return nil, nil
}

func (ps *proxyServer) Restore(ctx context.Context, request *pb.RestoreRequest) (*pb.RestoreReply, error) {
	return nil, nil
}

func main() {
	log.Printf("etcd-cluster-controller upload/download Proxy service")

	// Setup defaults for expected configuration keys
	viper.SetDefault("api-port", 8080)

	// Allow configuration to be passed via environment variables
	viper.SetEnvKeyReplacer(strings.NewReplacer("_", "-"))
	viper.AutomaticEnv()

	// Attempt to load a config file for configuration too
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/etcd-backup-restore-proxy/")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Print("Configuration file not present.")
		} else {
			log.Fatal(err, "Error reading configuration file")
		}
	}

	// Launch gRPC server
	grpcAddress := fmt.Sprintf(":%d", viper.GetInt("api-port"))
	log.Printf("Using %q as listen address for proxy server", grpcAddress)
	listener, err := net.Listen("tcp", grpcAddress)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterProxyServer(srv, &proxyServer{})
	if err := srv.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
