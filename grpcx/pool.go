package grpcx

import (
	"errors"
	"fmt"
	"sync"

	"google.golang.org/grpc"
)

// Pool manages a set of gRPC client connections keyed by address.
// It is safe for concurrent use.
//
// Dial options are only used when a connection is first created for a given
// address. Subsequent calls to Connect with the same address return the
// cached connection and ignore any new options.
//
// After Close is called, the pool is no longer usable and Connect will
// return an error.
type Pool struct {
	mu     sync.Mutex
	conns  map[string]*grpc.ClientConn
	closed bool
}

// NewPool returns a new empty connection pool.
func NewPool() *Pool {
	return &Pool{
		conns: make(map[string]*grpc.ClientConn),
	}
}

// Connect returns an existing connection for the given address or creates a
// new one if none exists. All supplied dial options are passed to grpc.NewClient
// only when a new connection is created; they are ignored on cache hits.
//
// The returned connection is owned by the pool. Callers must not close it
// directly; use Pool.Close to close all connections.
func (p *Pool) Connect(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, fmt.Errorf("grpcx: address cannot be empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("grpcx: pool is closed")
	}

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

// Close closes all connections in the pool. After Close, the pool is no
// longer usable and Connect will return an error.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	var errs []error
	for addr, conn := range p.conns {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("grpcx: failed to close conn %q: %w", addr, err))
		}
	}
	p.conns = make(map[string]*grpc.ClientConn)
	return errors.Join(errs...)
}

// Len returns the number of connections currently in the pool.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}
