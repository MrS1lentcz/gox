# gox

Lightweight Go utility packages for building gRPC services with Sentry integration.

## Installation

```bash
go get github.com/mrs1lentcz/gox
```

## Packages

### `grpcx` — gRPC client utilities

**Connection pool** — thread-safe, get-or-create connection pool:

```go
pool := grpcx.NewPool()
defer pool.Close()

conn, err := pool.Connect("localhost:50051",
    grpc.WithTransportCredentials(insecure.NewCredentials()),
)
```

**Metadata helpers** — extract values from incoming gRPC metadata:

```go
token, err := grpcx.GetMetadataValue(ctx, "authorization")

if grpcx.HasMetadataKey(ctx, "x-request-id") {
    // ...
}
```

### `grpcx/server` — gRPC server bootstrap

Starts a gRPC server with health checking, reflection, and graceful shutdown:

```go
err := server.Run(ctx, server.Config{
    Addr:       ":50051",
    Reflection: true,
    Health:     true,
    ServerOptions: []grpc.ServerOption{
        grpc.MaxRecvMsgSize(16 * 1024 * 1024),
    },
    RegisterServices: func(s *grpc.Server) error {
        pb.RegisterMyServiceServer(s, &myService{})
        return nil
    },
    OnShutdown: func() {
        db.Close()
    },
})
```

The server stops gracefully on `SIGINT`, `SIGTERM`, or context cancellation.

### `sentryx` — Sentry integration for gRPC

**Initialization:**

```go
if err := sentryx.Init(dsn, "my-service"); err != nil {
    log.Fatal(err)
}
defer sentryx.Flush(2 * time.Second)
```

**gRPC interceptors** — distributed tracing with automatic error capture:

```go
srv := grpc.NewServer(
    grpc.ChainUnaryInterceptor(sentryx.UnaryServerInterceptor()),
    grpc.ChainStreamInterceptor(sentryx.StreamServerInterceptor()),
)
```

The interceptors propagate `sentry-trace` and `baggage` headers from incoming metadata and only report server-side errors (`Internal`, `Unknown`, `Unavailable`, `DataLoss`, `ResourceExhausted`) to Sentry.

## License

MIT
