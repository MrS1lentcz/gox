package grpcx

import (
	"sync"
	"testing"

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
	p.Close()
}

func TestPool_Connect_ReusesConnection(t *testing.T) {
	p := NewPool()
	defer p.Close()

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
	defer p.Close()

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

func TestPool_Close_ClearsPool(t *testing.T) {
	p := NewPool()
	opts := grpc.WithTransportCredentials(insecure.NewCredentials())
	p.Connect("passthrough:///localhost:0", opts)

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
	conn.Close()

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
	defer p.Close()

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
