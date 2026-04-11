package grpcx

import (
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNewPool(t *testing.T) {
	p := NewPool()
	if p == nil {
		t.Fatal("NewPool returned nil")
	}
	if p.Len() != 0 {
		t.Fatalf("new pool should be empty, got %d", p.Len())
	}
}

func TestPool_Connect_EmptyAddr(t *testing.T) {
	p := NewPool()
	_, err := p.Connect("")
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestPool_Connect_CreatesConnection(t *testing.T) {
	p := NewPool()
	conn, err := p.Connect("passthrough:///localhost:0", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	if p.Len() != 1 {
		t.Fatalf("expected pool size 1, got %d", p.Len())
	}
	_ = p.Close()
}

func TestPool_Connect_ReusesConnection(t *testing.T) {
	p := NewPool()
	defer func() { _ = p.Close() }()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn1, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	conn2, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn1 != conn2 {
		t.Fatal("expected same connection for same address")
	}
	if p.Len() != 1 {
		t.Fatalf("expected pool size 1, got %d", p.Len())
	}
}

func TestPool_Connect_DifferentAddresses(t *testing.T) {
	p := NewPool()
	defer func() { _ = p.Close() }()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn1, _ := p.Connect("passthrough:///localhost:0", opts)
	conn2, _ := p.Connect("passthrough:///localhost:1", opts)
	if conn1 == conn2 {
		t.Fatal("expected different connections for different addresses")
	}
	if p.Len() != 2 {
		t.Fatalf("expected pool size 2, got %d", p.Len())
	}
}

func TestPool_Connect_StoresOpts(t *testing.T) {
	p := NewPool()
	defer func() { _ = p.Close() }()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	_, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p.mu.Lock()
	entry := p.conns["passthrough:///localhost:0"]
	p.mu.Unlock()

	if len(entry.opts) != 1 {
		t.Fatalf("expected 1 stored dial option, got %d", len(entry.opts))
	}
}

func TestPool_Close_ClearsPool(t *testing.T) {
	p := NewPool()
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	_, _ = p.Connect("passthrough:///localhost:0", opts)

	if err := p.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Len() != 0 {
		t.Fatalf("expected empty pool after close, got %d", p.Len())
	}
}

func TestPool_Close_Error(t *testing.T) {
	p := NewPool()
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, _ := p.Connect("passthrough:///localhost:0", opts)
	// Close the conn directly so Pool.Close will encounter an error.
	_ = conn.Close()

	if err := p.Close(); err == nil {
		t.Fatal("expected error when closing already-closed connection")
	}
	if p.Len() != 0 {
		t.Fatalf("pool should be empty after Close, got %d", p.Len())
	}
}

func TestPool_Close_Empty(t *testing.T) {
	p := NewPool()
	if err := p.Close(); err != nil {
		t.Fatalf("unexpected error closing empty pool: %v", err)
	}
}

func TestPool_Connect_AfterClose(t *testing.T) {
	p := NewPool()
	_ = p.Close()

	_, err := p.Connect("passthrough:///localhost:0", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err == nil {
		t.Fatal("expected error when connecting on closed pool")
	}
}

func TestPool_Connect_DialError(t *testing.T) {
	p := NewPool()
	// grpc.NewClient fails when no transport credentials are set.
	_, err := p.Connect("localhost:0")
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
	if p.Len() != 0 {
		t.Fatalf("pool should be empty after dial error, got %d", p.Len())
	}
}

func TestPool_ConcurrentAccess(t *testing.T) {
	p := NewPool()
	defer func() { _ = p.Close() }()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := p.Connect("passthrough:///localhost:0", opts)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if p.Len() != 1 {
		t.Fatalf("expected pool size 1, got %d", p.Len())
	}
}

func TestPool_Reconnect(t *testing.T) {
	p := NewPool()
	defer func() { _ = p.Close() }()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn1, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	conn2, err := p.Reconnect("passthrough:///localhost:0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conn1 == conn2 {
		t.Fatal("expected new connection after reconnect")
	}
	if p.Len() != 1 {
		t.Fatalf("expected pool size 1, got %d", p.Len())
	}
}

func TestPool_Reconnect_UnknownAddr(t *testing.T) {
	p := NewPool()
	defer func() { _ = p.Close() }()

	_, err := p.Reconnect("passthrough:///unknown:0")
	if err == nil {
		t.Fatal("expected error for unknown address")
	}
}

func TestPool_Reconnect_AfterClose(t *testing.T) {
	p := NewPool()
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	_, _ = p.Connect("passthrough:///localhost:0", opts)
	_ = p.Close()

	_, err := p.Reconnect("passthrough:///localhost:0")
	if err == nil {
		t.Fatal("expected error on closed pool")
	}
}

func TestPool_Reconnect_DialError(t *testing.T) {
	p := NewPool()
	defer func() { _ = p.Close() }()

	// First connect with valid opts.
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	_, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Replace opts with invalid ones to force redial failure.
	p.mu.Lock()
	p.conns["passthrough:///localhost:0"].opts = nil
	p.mu.Unlock()

	_, err = p.Reconnect("passthrough:///localhost:0")
	if err == nil {
		t.Fatal("expected error for redial failure")
	}
	if p.Len() != 0 {
		t.Fatalf("expected pool to remove failed entry, got %d", p.Len())
	}
}

func TestPool_Reconnect_Concurrent(t *testing.T) {
	p := NewPool()
	defer func() { _ = p.Close() }()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	_, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, rerr := p.Reconnect("passthrough:///localhost:0")
			if rerr != nil {
				t.Errorf("unexpected error: %v", rerr)
			}
		}()
	}
	wg.Wait()

	if p.Len() != 1 {
		t.Fatalf("expected pool size 1, got %d", p.Len())
	}
}

func TestPool_EnableHealthWatch(t *testing.T) {
	p := NewPool()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	_, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p.EnableHealthWatch(10 * time.Millisecond)
	// Second call should be a no-op.
	p.EnableHealthWatch(10 * time.Millisecond)

	// Let the health check run a few times.
	time.Sleep(50 * time.Millisecond)

	// Connection should still be alive (passthrough connections are Idle, not failed).
	if p.Len() != 1 {
		t.Fatalf("expected pool size 1, got %d", p.Len())
	}

	_ = p.Close()
}

func TestPool_EnableHealthWatch_RemovesFailedConn(t *testing.T) {
	p := NewPool()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close the connection directly to simulate a dead connection.
	// GetState will return Shutdown.
	_ = conn.Close()

	// Manually trigger check instead of waiting for ticker.
	p.checkConnections()

	// The broken conn should have been reconnected or removed.
	// Since opts are valid, it should reconnect successfully.
	if p.Len() != 1 {
		t.Fatalf("expected pool size 1 after health check reconnect, got %d", p.Len())
	}

	// The connection should be different from the closed one.
	p.mu.Lock()
	entry := p.conns["passthrough:///localhost:0"]
	p.mu.Unlock()

	if entry.conn == conn {
		t.Fatal("expected new connection after health check reconnect")
	}

	_ = p.Close()
}

func TestPool_EnableHealthWatch_RemovesOnDialError(t *testing.T) {
	p := NewPool()

	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	conn, err := p.Connect("passthrough:///localhost:0", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close conn and remove opts to simulate dial failure on reconnect.
	_ = conn.Close()
	p.mu.Lock()
	p.conns["passthrough:///localhost:0"].opts = nil
	p.mu.Unlock()

	p.checkConnections()

	if p.Len() != 0 {
		t.Fatalf("expected empty pool after failed reconnect, got %d", p.Len())
	}

	_ = p.Close()
}

func TestPool_Close_StopsHealthWatch(t *testing.T) {
	p := NewPool()
	p.EnableHealthWatch(10 * time.Millisecond)
	_ = p.Close()

	// Verify stop channel is nil after close.
	p.mu.Lock()
	stopped := p.stop == nil
	p.mu.Unlock()

	if !stopped {
		t.Fatal("expected health watch to be stopped after Close")
	}
}
