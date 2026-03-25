package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

// DefaultHealthServer returns a health.Server with the root service set to SERVING.
// The returned server can be further configured before passing to Config.
func DefaultHealthServer() *health.Server {
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	return hs
}

// Config holds the configuration for a gRPC server.
type Config struct {
	// Addr is the TCP address to listen on (e.g. ":50051").
	Addr string

	// Reflection enables the gRPC server reflection service.
	Reflection bool

	// HealthServer, when set, is registered on the gRPC server.
	// The caller retains full control over service statuses.
	// If nil, no health service is registered.
	HealthServer *health.Server

	// ShutdownTimeout is the maximum time to wait for in-flight RPCs
	// to complete during graceful shutdown. If zero, GracefulStop
	// blocks indefinitely. After the timeout, the server is forcefully
	// stopped.
	ShutdownTimeout time.Duration

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
// SIGINT or SIGTERM is received, or the context is cancelled. It then
// gracefully stops the server (subject to ShutdownTimeout) and calls
// OnShutdown if set.
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

	if cfg.HealthServer != nil {
		grpc_health_v1.RegisterHealthServer(srv, cfg.HealthServer)
	}

	signals := cfg.signals
	if signals == nil {
		stopChan := make(chan os.Signal, 1)
		signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(stopChan)
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
		gracefulStop(srv, cfg.ShutdownTimeout)
		err = nil
	case <-ctx.Done():
		gracefulStop(srv, cfg.ShutdownTimeout)
		err = ctx.Err()
	}

	if cfg.OnShutdown != nil {
		cfg.OnShutdown()
	}

	return err
}

// gracefulStop attempts a graceful shutdown within the timeout.
// If timeout is zero, it blocks indefinitely.
func gracefulStop(srv *grpc.Server, timeout time.Duration) {
	if timeout == 0 {
		srv.GracefulStop()
		return
	}

	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		srv.Stop()
	}
}
