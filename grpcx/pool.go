package grpcx

import (
	"fmt"
	"sync"

	"google.golang.org/grpc"
)

// Pool manages a set of gRPC client connections keyed by address.
// It is safe for concurrent use.
type Pool struct {
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

// NewPool returns a new empty connection pool.
func NewPool() *Pool {
	return &Pool{
		conns: make(map[string]*grpc.ClientConn),
	}
}

// Connect returns an existing connection for the given address or creates a
// new one if none exists. All supplied dial options are passed to grpc.NewClient.
func (p *Pool) Connect(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, fmt.Errorf("grpcx: address cannot be empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok := p.conns[addr]; ok {
		return conn, nil
	}

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpcx: failed to dial %q: %w", addr, err)
	}

	p.conns[addr] = conn
	return conn, nil
}

// Close closes all connections in the pool and resets it to an empty state.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	for addr, conn := range p.conns {
		if err := conn.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("grpcx: failed to close conn %q: %w", addr, err)
		}
	}
	p.conns = make(map[string]*grpc.ClientConn)
	return firstErr
}

// Len returns the number of connections currently in the pool.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}
