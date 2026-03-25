package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// Config holds the configuration for a gRPC server.
type Config struct {
	// Addr is the TCP address to listen on (e.g. ":50051").
	Addr string

	// Reflection enables the gRPC server reflection service.
	Reflection bool

	// Health enables the gRPC health checking service.
	Health bool

	// ServerOptions are additional gRPC server options.
	ServerOptions []grpc.ServerOption

	// RegisterServices is called to register gRPC services on the server.
	RegisterServices func(s *grpc.Server) error

	// OnShutdown is called after the server has stopped. May be nil.
	OnShutdown func()

	// signals is used for testing to inject a signal channel.
	signals <-chan os.Signal
}

// Run starts a gRPC server with the given configuration and blocks until a
// SIGINT or SIGTERM is received. It then gracefully stops the server and
// calls OnShutdown if set.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Addr == "" {
		return fmt.Errorf("grpcx/server: address is required")
	}
	if cfg.RegisterServices == nil {
		return fmt.Errorf("grpcx/server: RegisterServices is required")
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("grpcx/server: failed to listen on %s: %w", cfg.Addr, err)
	}

	srv := grpc.NewServer(cfg.ServerOptions...)

	if err = cfg.RegisterServices(srv); err != nil {
		listener.Close()
		return fmt.Errorf("grpcx/server: failed to register services: %w", err)
	}

	if cfg.Reflection {
		reflection.Register(srv)
	}

	if cfg.Health {
		hs := health.NewServer()
		grpc_health_v1.RegisterHealthServer(srv, hs)
		hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	}

	signals := cfg.signals
	if signals == nil {
		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
		signals = stopChan
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(listener)
	}()

	select {
	case err = <-serveErr:
		// Server stopped on its own (e.g. listener closed).
	case <-signals:
		srv.GracefulStop()
		err = nil
	case <-ctx.Done():
		srv.GracefulStop()
		err = ctx.Err()
	}

	if cfg.OnShutdown != nil {
		cfg.OnShutdown()
	}

	return err
}
