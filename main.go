package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

var (
	port        int
	tlsKeyPath  string
	tlsCertPath string
)

func main() {
	flag.StringVar(&tlsKeyPath, "tls-key", os.Getenv("TLS_KEY"), "TLS key file path")
	flag.StringVar(&tlsCertPath, "tls-cert", os.Getenv("TLS_CERT"), "TLS cert file path")
	flag.IntVar(&port, "port", 8000, "Listen port")
	flag.Parse()

	if err := runServer(); err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}
}

func runServer() error {
	var serverOptions []grpc.ServerOption
	if tlsKeyPath != "" || tlsCertPath != "" {
		cred, err := credentials.NewServerTLSFromFile(tlsCertPath, tlsKeyPath)
		if err != nil {
			return fmt.Errorf("failed to create transport credential: %w", err)
		}
		serverOptions = append(serverOptions, grpc.Creds(cred))
	}
	server := grpc.NewServer(serverOptions...)
	grpc_health_v1.RegisterHealthServer(server, &HealthServer{})
	reflection.Register(server)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	listner, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("failed to listen port: %w", err)
	}
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	slog.Info("Start server", "listen_on", listner.Addr().String())
	if err := server.Serve(listner); err != nil {
		return err
	}
	return nil
}

type HealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
}

func (h *HealthServer) Check(_ context.Context, in *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{
		Status: grpc_health_v1.HealthCheckResponse_SERVING,
	}, nil
}

func (h *HealthServer) Watch(req *grpc_health_v1.HealthCheckRequest, srv grpc_health_v1.Health_WatchServer) error {
	ctx := srv.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
		if err := srv.Send(&grpc_health_v1.HealthCheckResponse{
			Status: grpc_health_v1.HealthCheckResponse_SERVING,
		}); err != nil {
			return err
		}
	}
}
