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

Dial options are applied only when a connection is first created for a given address. The pool is not reusable after `Close`.

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
    Addr:            ":50051",
    Reflection:      true,
    HealthServer:    server.DefaultHealthServer(),
    ShutdownTimeout: 10 * time.Second,
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

The server stops gracefully on `SIGINT`, `SIGTERM`, or context cancellation. If `ShutdownTimeout` is set, the server is forcefully stopped after the timeout.

### `sentryx` — Sentry integration for gRPC

**Initialization:**

```go
// Quick start with sensible defaults:
err := sentryx.Init(sentryx.DefaultClientOptions(dsn, "my-service"))

// Or fully customized:
opts := sentryx.DefaultClientOptions(dsn, "my-service")
opts.Environment = "production"
opts.Release = "v1.0.0"
opts.TracesSampleRate = 0.1
err := sentryx.Init(opts)

if err != nil {
    log.Fatal(err)
}
defer sentryx.Flush(2 * time.Second)
```

**gRPC interceptors** — distributed tracing with panic recovery and automatic error capture:

```go
srv := grpc.NewServer(
    grpc.ChainUnaryInterceptor(sentryx.UnaryServerInterceptor()),
    grpc.ChainStreamInterceptor(sentryx.StreamServerInterceptor()),
)
```

The interceptors:
- Propagate `sentry-trace` and `baggage` headers for distributed tracing
- Recover panics and report them to Sentry
- Report server-side errors (`Internal`, `Unknown`, `Unavailable`, `DataLoss`, `ResourceExhausted`) by default
- Map gRPC status codes to proper Sentry span statuses

**Custom error filter:**

```go
sentryx.UnaryServerInterceptor(
    sentryx.WithErrorFilter(func(err error) bool {
        // Only capture Internal errors.
        return status.Code(err) == codes.Internal
    }),
)
```

## License

MIT
