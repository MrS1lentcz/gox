// Package entx provides utilities for working with entgo.io/ent,
// including automated migration generation using ephemeral Docker containers.
package entx

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	entSql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
)

// Env is a Docker environment variable in "KEY=VALUE" format.
type Env = string

// ContainerPort is the port exposed by the database inside the container (e.g. "5432").
type ContainerPort = string

// DSN is a database connection string.
type DSN = string

// Config configures the MakeMigrations function.
type Config struct {
	// Driver is the database/sql driver name (e.g. "postgres", "mysql").
	Driver string

	// Image is the Docker image to use for the ephemeral database container
	// (e.g. "postgres:16-alpine", "mysql:8").
	Image string

	// Dir is the output directory for migration SQL files.
	Dir string

	// Name is an optional migration name. When set, it is included in the
	// filename (e.g. "0001_add_users_20260329120000.sql").
	Name string

	// Tables is the ent schema table list, typically the generated migrate.Tables variable.
	Tables []*schema.Table

	// Setup returns the Docker environment variables, the container port to
	// map, and the DSN for the given host port. Use the provided helpers
	// (Postgres, MySQL) or supply a custom function.
	Setup func(hostPort string) ([]Env, ContainerPort, DSN)

	// OnConnect is called after the database is ready with an open connection.
	// Use it for setup that cannot be done via environment variables alone
	// (e.g. CREATE DATABASE on SQL Server). Optional.
	OnConnect func(ctx context.Context, db *sql.DB) error
}

// Swappable functions for testing.
var (
	allocatePort     = defaultAllocatePort
	runContainer     = defaultRunContainer
	destroyContainer = defaultDestroyContainer
	pingDatabase     = defaultPingDatabase
	openDatabase     = defaultOpenDatabase
	generateSchema   = defaultGenerateSchema
)

// MakeMigrations generates a SQL migration file by diffing the provided ent
// schema tables against an empty database running in an ephemeral Docker
// container. It returns the path to the generated file, or an empty string
// when no schema changes are detected.
func MakeMigrations(ctx context.Context, cfg Config) (string, error) {
	if cfg.Driver == "" {
		return "", fmt.Errorf("entx: driver is required")
	}
	if cfg.Image == "" {
		return "", fmt.Errorf("entx: image is required")
	}
	if cfg.Dir == "" {
		return "", fmt.Errorf("entx: dir is required")
	}
	if len(cfg.Tables) == 0 {
		return "", fmt.Errorf("entx: tables are required")
	}
	if cfg.Setup == nil {
		return "", fmt.Errorf("entx: setup is required")
	}

	hostPort, err := allocatePort()
	if err != nil {
		return "", err
	}

	env, containerPort, dsn := cfg.Setup(hostPort)

	containerName := fmt.Sprintf("entx-%d", time.Now().UnixNano())
	if err := runContainer(ctx, containerName, cfg.Image, containerPort, hostPort, env); err != nil {
		return "", err
	}
	defer destroyContainer(containerName)

	if err := pingDatabase(ctx, cfg.Driver, dsn, 60*time.Second); err != nil {
		return "", err
	}

	if cfg.OnConnect != nil {
		db, err := openDatabase(cfg.Driver, dsn)
		if err != nil {
			return "", fmt.Errorf("entx: open connection for OnConnect: %w", err)
		}
		if err := cfg.OnConnect(ctx, db); err != nil {
			db.Close()
			return "", fmt.Errorf("entx: on connect: %w", err)
		}
		db.Close()
	}

	sqlContent, err := generateSchema(ctx, cfg.Driver, dsn, cfg.Tables)
	if err != nil {
		return "", err
	}

	if sqlContent == "" {
		return "", nil
	}

	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return "", fmt.Errorf("entx: create migrations dir: %w", err)
	}

	seq, err := nextSequence(cfg.Dir)
	if err != nil {
		return "", err
	}

	filename := migrationFilename(seq, cfg.Name)
	path := filepath.Join(cfg.Dir, filename)
	if err := os.WriteFile(path, []byte(sqlContent), 0o644); err != nil {
		return "", fmt.Errorf("entx: write migration file: %w", err)
	}

	return path, nil
}

func defaultAllocatePort() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("entx: allocate free port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return fmt.Sprintf("%d", port), nil
}

// migrationFilename builds a filename like "0001_add_users_20260329120000.sql".
func migrationFilename(seq int, name string) string {
	ts := time.Now().UTC().Format("20060102150405")
	if name == "" {
		return fmt.Sprintf("%04d_%s.sql", seq, ts)
	}
	safe := filepath.Base(name)
	return fmt.Sprintf("%04d_%s_%s.sql", seq, safe, ts)
}

// nextSequence scans the directory for existing migration files and returns
// the next sequence number.
func nextSequence(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("entx: read migrations dir: %w", err)
	}

	max := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(e.Name(), "%04d", &n); err == nil && n > max {
			max = n
		}
	}
	return max + 1, nil
}

func defaultRunContainer(ctx context.Context, name, image, containerPort, hostPort string, env []string) error {
	args := []string{"run", "-d", "--name", name, "-p", hostPort + ":" + containerPort}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, image)

	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("entx: start container: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func defaultDestroyContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", name).Run()
}

// defaultPingDatabase polls the database with sql.Open + Ping until it responds
// or the timeout expires.
func defaultPingDatabase(ctx context.Context, driver, dsn string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("entx: context canceled while waiting for database: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("entx: database not ready within %v", timeout)
		}

		db, err := sql.Open(driver, dsn)
		if err == nil {
			err = db.PingContext(ctx)
			db.Close()
			if err == nil {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("entx: context canceled while waiting for database: %w", ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func defaultOpenDatabase(driver, dsn string) (*sql.DB, error) {
	return sql.Open(driver, dsn)
}

func defaultGenerateSchema(ctx context.Context, driver, dsn string, tables []*schema.Table) (string, error) {
	drv, err := entSql.Open(driver, dsn)
	if err != nil {
		return "", fmt.Errorf("entx: open connection: %w", err)
	}
	defer drv.Close()

	var buf strings.Builder
	writeDriver := &schema.WriteDriver{Writer: &buf, Driver: drv}
	migrate, err := schema.NewMigrate(writeDriver)
	if err != nil {
		return "", fmt.Errorf("entx: create migrator: %w", err)
	}
	if err := migrate.Create(ctx, tables...); err != nil {
		return "", fmt.Errorf("entx: generate schema: %w", err)
	}

	return buf.String(), nil
}
