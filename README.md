# gox

[![CI](https://github.com/MrS1lentcz/gox/actions/workflows/ci.yml/badge.svg)](https://github.com/MrS1lentcz/gox/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/MrS1lentcz/gox/branch/main/graph/badge.svg)](https://codecov.io/gh/MrS1lentcz/gox)
[![Go Reference](https://pkg.go.dev/badge/github.com/mrs1lentcz/gox.svg)](https://pkg.go.dev/github.com/mrs1lentcz/gox)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Lightweight Go extension packages for people who are building services using [gRPC](https://grpc.io/), [Sentry](https://sentry.io/), and [Ent](https://entgo.io/) (+[Atlas](https://atlasgo.io/)). Docker required for migrations.

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

### `errorx` — configurable error reporting

Parses a config string and returns a `Reporter` that sends errors either to Sentry or stderr:

```go
// Quick bootstrap — parses config, calls log.Fatal on error:
errorx.MustInit(os.Getenv("ERROR_LOGGER"))
defer errorx.CloseReporter()

// Report errors anywhere in the app:
errorx.ReportError(err)
```

Or use the `Reporter` interface directly:

```go
reporter, err := errorx.New(os.Getenv("ERROR_LOGGER"))
if err != nil {
    log.Fatal(err)
}
defer reporter.Close()

reporter.Report(err)
```

**Custom error filter:**

```go
sentryx.UnaryServerInterceptor(
    sentryx.WithErrorFilter(func(err error) bool {
        // Only capture Internal errors.
        return status.Code(err) == codes.Internal
    }),
)
```

### `entx` — automated ent migration generation

Generates SQL migration files by diffing your ent schema against an empty database running in an ephemeral Docker container. Docker is required.

**Quick start with Postgres:**

```go
import (
    "context"

    _ "github.com/lib/pq"

    "github.com/mrs1lentcz/gox/entx"
    "myproject/ent/migrate"
)

func main() {
    path, err := entx.MakeMigrations(context.Background(),
        entx.Postgres18("ent/migrate/migrations", "add_users", migrate.Tables),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Migration written to", path)
}
```

This will:
1. Ask the OS for a free port
2. Start an ephemeral `postgres:18-alpine` container on that port
3. Connect, diff your ent schema against the empty database
4. Write a timestamped SQL file (e.g. `0001_add_users_20260329120000.sql`)
5. Remove the container

**Built-in helpers:**

| Helper | Image |
|---|---|
| `entx.Postgres17(dir, name, tables)` | `postgres:17-alpine` |
| `entx.Postgres18(dir, name, tables)` | `postgres:18-alpine` |
| `entx.Postgres(version, dir, name, tables)` | `postgres:<version>-alpine` |
| `entx.MySQL84(dir, name, tables)` | `mysql:8.4` |
| `entx.MySQL9(dir, name, tables)` | `mysql:9` |
| `entx.MySQL(version, dir, name, tables)` | `mysql:<version>` |

**Custom database setup:**

```go
entx.MakeMigrations(ctx, entx.Config{
    Driver: "postgres",
    Image:  "postgres:16-alpine",
    Dir:    "migrations",
    Name:   "init",
    Tables: migrate.Tables,
    Setup: func(hostPort string) ([]entx.Env, entx.ContainerPort, entx.DSN) {
        return []entx.Env{
                "POSTGRES_USER=myuser",
                "POSTGRES_PASSWORD=mypass",
                "POSTGRES_DB=mydb",
            },
            "5432",
            entx.DSN("postgres://myuser:mypass@localhost:" + hostPort + "/mydb?sslmode=disable")
    },
    OnConnect: func(ctx context.Context, db *sql.DB) error {
        // Run any post-startup SQL (e.g. CREATE EXTENSION).
        _, err := db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")
        return err
    },
})
```

## License

MIT
