package grpcx

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type poolEntry struct {
	conn *grpc.ClientConn
	opts []grpc.DialOption
}

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
	conns  map[string]*poolEntry
	closed bool
	stop   chan struct{} // signals health watch to stop
}

// NewPool returns a new empty connection pool.
func NewPool() *Pool {
	return &Pool{
		conns: make(map[string]*poolEntry),
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

	if entry, ok := p.conns[addr]; ok {
		return entry.conn, nil
	}

	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpcx: failed to dial %q: %w", addr, err)
	}

	p.conns[addr] = &poolEntry{conn: conn, opts: opts}
	return conn, nil
}

// Reconnect closes the existing connection for the given address and creates
// a new one using the same dial options. Returns the new connection.
// Returns an error if no connection exists for the address.
func (p *Pool) Reconnect(addr string) (*grpc.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, fmt.Errorf("grpcx: pool is closed")
	}

	entry, ok := p.conns[addr]
	if !ok {
		return nil, fmt.Errorf("grpcx: no connection for %q", addr)
	}

	_ = entry.conn.Close()

	conn, err := grpc.NewClient(addr, entry.opts...)
	if err != nil {
		delete(p.conns, addr)
		return nil, fmt.Errorf("grpcx: failed to redial %q: %w", addr, err)
	}

	entry.conn = conn
	return conn, nil
}

// EnableHealthWatch starts a background goroutine that periodically checks
// the connectivity state of all connections in the pool. Connections in
// TransientFailure or Shutdown state are closed and reconnected using their
// original dial options. If reconnect fails, the connection is removed from
// the pool.
//
// The goroutine is stopped when Close is called.
func (p *Pool) EnableHealthWatch(interval time.Duration) {
	p.mu.Lock()
	if p.stop != nil {
		p.mu.Unlock()
		return
	}
	p.stop = make(chan struct{})
	stop := p.stop
	p.mu.Unlock()

	go p.healthLoop(interval, stop)
}

func (p *Pool) healthLoop(interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			p.checkConnections()
		}
	}
}

func (p *Pool) checkConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for addr, entry := range p.conns {
		state := entry.conn.GetState()
		if state == connectivity.TransientFailure || state == connectivity.Shutdown {
			_ = entry.conn.Close()

			conn, err := grpc.NewClient(addr, entry.opts...)
			if err != nil {
				delete(p.conns, addr)
				continue
			}
			entry.conn = conn
		}
	}
}

// Close closes all connections in the pool and stops the health watch
// goroutine if running. After Close, the pool is no longer usable and
// Connect will return an error.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	if p.stop != nil {
		close(p.stop)
		p.stop = nil
	}

	var errs []error
	for addr, entry := range p.conns {
		if err := entry.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("grpcx: failed to close conn %q: %w", addr, err))
		}
	}
	p.conns = make(map[string]*poolEntry)
	return errors.Join(errs...)
}

// Len returns the number of connections currently in the pool.
func (p *Pool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}
