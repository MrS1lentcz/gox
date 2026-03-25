package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func TestRun_MissingAddr(t *testing.T) {
	err := Run(context.Background(), Config{
		RegisterServices: func(s *grpc.Server) error { return nil },
	})
	if err == nil {
		t.Fatal("expected error for missing address")
	}
}

func TestRun_MissingRegisterServices(t *testing.T) {
	err := Run(context.Background(), Config{Addr: ":0"})
	if err == nil {
		t.Fatal("expected error for missing RegisterServices")
	}
}

func TestRun_RegisterServicesError(t *testing.T) {
	addr := freePort(t)
	err := Run(context.Background(), Config{
		Addr: addr,
		RegisterServices: func(s *grpc.Server) error {
			return fmt.Errorf("register failed")
		},
	})
	if err == nil {
		t.Fatal("expected error from RegisterServices")
	}
}

func TestRun_GracefulShutdownViaSignal(t *testing.T) {
	addr := freePort(t)
	sigChan := make(chan os.Signal, 1)
	shutdownCalled := false

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Addr: addr,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
			OnShutdown: func() {
				shutdownCalled = true
			},
			signals: sigChan,
		})
	}()

	waitForServer(t, addr)
	sigChan <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}

	if !shutdownCalled {
		t.Fatal("OnShutdown was not called")
	}
}

func TestRun_GracefulShutdownViaContext(t *testing.T) {
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Addr: addr,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
			signals: make(<-chan os.Signal),
		})
	}()

	waitForServer(t, addr)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for shutdown")
	}
}

func TestRun_WithReflection(t *testing.T) {
	addr := freePort(t)
	sigChan := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Addr:       addr,
			Reflection: true,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
			signals: sigChan,
		})
	}()

	waitForServer(t, addr)
	sigChan <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestDefaultHealthServer(t *testing.T) {
	hs := DefaultHealthServer()
	if hs == nil {
		t.Fatal("DefaultHealthServer returned nil")
	}
}

func TestRun_HealthServer(t *testing.T) {
	addr := freePort(t)
	sigChan := make(chan os.Signal, 1)
	hs := DefaultHealthServer()

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Addr:         addr,
			HealthServer: hs,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
			signals: sigChan,
		})
	}()

	waitForServer(t, addr)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := grpc_health_v1.NewHealthClient(conn)
	resp, err := client.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("expected SERVING, got %v", resp.Status)
	}

	sigChan <- os.Interrupt
	<-done
}

func TestRun_NilOnShutdown(t *testing.T) {
	addr := freePort(t)
	sigChan := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Addr: addr,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
			signals: sigChan,
		})
	}()

	waitForServer(t, addr)
	sigChan <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestRun_ShutdownTimeout(t *testing.T) {
	addr := freePort(t)
	sigChan := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Addr:            addr,
			ShutdownTimeout: 1 * time.Second,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
			signals: sigChan,
		})
	}()

	waitForServer(t, addr)
	sigChan <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestRun_ShutdownTimeoutViaContext(t *testing.T) {
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Addr:            addr,
			ShutdownTimeout: 1 * time.Second,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
			signals: make(<-chan os.Signal),
		})
	}()

	waitForServer(t, addr)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestRun_ListenError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	addr := l.Addr().String()
	defer func() { _ = l.Close() }()

	err = Run(context.Background(), Config{
		Addr: addr,
		RegisterServices: func(s *grpc.Server) error {
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected listen error")
	}
}

func TestRun_SignalShutdown(t *testing.T) {
	addr := freePort(t)
	sigChan := make(chan os.Signal, 1)

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Addr: addr,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
			signals: sigChan,
		})
	}()

	waitForServer(t, addr)
	sigChan <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestRun_DefaultSignals(t *testing.T) {
	addr := freePort(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Addr: addr,
			RegisterServices: func(s *grpc.Server) error {
				return nil
			},
		})
	}()

	waitForServer(t, addr)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestRun_ServerStopsBeforeSignal(t *testing.T) {
	addr := freePort(t)

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Addr: addr,
			RegisterServices: func(s *grpc.Server) error {
				go func() {
					time.Sleep(100 * time.Millisecond)
					s.Stop()
				}()
				return nil
			},
			signals: make(<-chan os.Signal),
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestGracefulStop_ZeroTimeout(t *testing.T) {
	srv := grpc.NewServer()
	gracefulStop(srv, 0)
}

func TestGracefulStop_WithTimeout(t *testing.T) {
	srv := grpc.NewServer()
	gracefulStop(srv, 1*time.Second)
}

func TestGracefulStop_TimeoutForcesStop(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	addr := l.Addr().String()

	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	))

	hs := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, hs)

	go func() { _ = srv.Serve(l) }()
	waitForServer(t, addr)

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	go func() {
		_, _ = grpc_health_v1.NewHealthClient(conn).Check(context.Background(), &grpc_health_v1.HealthCheckRequest{})
	}()

	time.Sleep(100 * time.Millisecond)

	gracefulStop(srv, 100*time.Millisecond)
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server at %s not ready in time", addr)
}
