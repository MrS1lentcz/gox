package entx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql/schema"
	"entgo.io/ent/schema/field"
)

// stubOpenDatabase returns a real *sql.DB backed by a no-op driver so that
// Close() and other methods don't panic.
func stubOpenDatabase(_, _ string) (*sql.DB, error) {
	return sql.Open("entx_stub", "")
}

type stubDBDriver struct{}

func (d stubDBDriver) Open(_ string) (driver.Conn, error) { return stubConn{}, nil }

type stubConn struct{}

func (c stubConn) Prepare(_ string) (driver.Stmt, error) { return nil, fmt.Errorf("stub") }
func (c stubConn) Close() error                          { return nil }
func (c stubConn) Begin() (driver.Tx, error)              { return nil, fmt.Errorf("stub") }

type stubFailPingDriver struct{}

func (d stubFailPingDriver) Open(_ string) (driver.Conn, error) { return stubFailPingConn{}, nil }

type stubFailPingConn struct{ stubConn }

func (c stubFailPingConn) Ping(_ context.Context) error { return fmt.Errorf("ping failed") }

func init() {
	sql.Register("entx_stub", stubDBDriver{})
	sql.Register("entx_stub_fail_ping", stubFailPingDriver{})
}

var testTables = func() []*schema.Table {
	t := []*schema.Table{
		{
			Name: "users",
			Columns: []*schema.Column{
				{Name: "id", Type: field.TypeInt, Increment: true},
			},
		},
	}
	t[0].PrimaryKey = []*schema.Column{t[0].Columns[0]}
	return t
}()

// stubFuncs replaces all swappable functions with test stubs and returns a
// cleanup function that restores the originals.
func stubFuncs(t *testing.T) {
	t.Helper()
	origAllocate := allocatePort
	origRun := runContainer
	origDestroy := destroyContainer
	origPing := pingDatabase
	origOpenDB := openDatabase
	origGenerate := generateSchema

	t.Cleanup(func() {
		allocatePort = origAllocate
		runContainer = origRun
		destroyContainer = origDestroy
		pingDatabase = origPing
		openDatabase = origOpenDB
		generateSchema = origGenerate
	})
}

func TestMakeMigrationsValidation(t *testing.T) {
	base := Config{
		Driver: "postgres",
		Image:  "postgres:17-alpine",
		Dir:    "/tmp",
		Tables: testTables,
		Setup: func(port string) ([]Env, ContainerPort, DSN) {
			return nil, "5432", "postgres://localhost:" + port
		},
	}

	tests := []struct {
		name   string
		modify func(Config) Config
	}{
		{"empty driver", func(c Config) Config { c.Driver = ""; return c }},
		{"empty image", func(c Config) Config { c.Image = ""; return c }},
		{"empty dir", func(c Config) Config { c.Dir = ""; return c }},
		{"empty tables", func(c Config) Config { c.Tables = nil; return c }},
		{"nil setup", func(c Config) Config { c.Setup = nil; return c }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.modify(base)
			_, err := MakeMigrations(context.Background(), cfg)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMakeMigrationsAllocatePortError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) {
		return "", fmt.Errorf("no ports")
	}

	_, err := MakeMigrations(context.Background(), Postgres17(t.TempDir(), "", testTables))
	if err == nil || !strings.Contains(err.Error(), "no ports") {
		t.Fatalf("expected allocate port error, got: %v", err)
	}
}

func TestMakeMigrationsStartContainerError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error {
		return fmt.Errorf("docker not found")
	}

	_, err := MakeMigrations(context.Background(), Postgres17(t.TempDir(), "", testTables))
	if err == nil || !strings.Contains(err.Error(), "docker not found") {
		t.Fatalf("expected container error, got: %v", err)
	}
}

func TestMakeMigrationsPingError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error {
		return fmt.Errorf("entx: database not ready within 60s")
	}

	_, err := MakeMigrations(context.Background(), Postgres17(t.TempDir(), "", testTables))
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected ping error, got: %v", err)
	}
}

func TestMakeMigrationsOnConnectOpenError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	openDatabase = func(_, _ string) (*sql.DB, error) {
		return nil, fmt.Errorf("connection refused")
	}

	cfg := Postgres17(t.TempDir(), "", testTables)
	cfg.OnConnect = func(_ context.Context, _ *sql.DB) error { return nil }

	_, err := MakeMigrations(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "OnConnect") {
		t.Fatalf("expected OnConnect open error, got: %v", err)
	}
}

func TestMakeMigrationsOnConnectCallbackError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	openDatabase = stubOpenDatabase

	cfg := Postgres17(t.TempDir(), "", testTables)
	cfg.OnConnect = func(_ context.Context, _ *sql.DB) error {
		return fmt.Errorf("create database failed")
	}

	_, err := MakeMigrations(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "on connect") {
		t.Fatalf("expected on connect error, got: %v", err)
	}
}

func TestMakeMigrationsGenerateSchemaError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "", fmt.Errorf("entx: open connection: connection refused")
	}

	_, err := MakeMigrations(context.Background(), Postgres17(t.TempDir(), "", testTables))
	if err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("expected generate schema error, got: %v", err)
	}
}

func TestMakeMigrationsEmptySchema(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "", nil
	}

	path, err := MakeMigrations(context.Background(), Postgres17(t.TempDir(), "", testTables))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path for empty schema, got: %q", path)
	}
}

func TestMakeMigrationsHappyPath(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "CREATE TABLE users (id INT PRIMARY KEY);", nil
	}

	dir := t.TempDir()
	path, err := MakeMigrations(context.Background(), Postgres17(dir, "init", testTables))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}

	base := filepath.Base(path)
	if !strings.HasPrefix(base, "0001_init_") {
		t.Errorf("filename = %q, want prefix 0001_init_", base)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(content) != "CREATE TABLE users (id INT PRIMARY KEY);" {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestMakeMigrationsHappyPathNoName(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "CREATE TABLE t (id INT);", nil
	}

	dir := t.TempDir()
	path, err := MakeMigrations(context.Background(), Postgres17(dir, "", testTables))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	base := filepath.Base(path)
	if !strings.HasPrefix(base, "0001_") {
		t.Errorf("filename = %q, want prefix 0001_", base)
	}
	// Should not have double underscore (no name segment).
	if strings.Contains(base, "__") {
		t.Errorf("filename should not have double underscore: %q", base)
	}
}

func TestMakeMigrationsSequenceIncrement(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "CREATE TABLE t (id INT);", nil
	}

	dir := t.TempDir()
	// Create first migration.
	_ = os.WriteFile(filepath.Join(dir, "0003_existing_20260101000000.sql"), []byte("x"), 0o644)

	path, err := MakeMigrations(context.Background(), Postgres17(dir, "next", testTables))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	base := filepath.Base(path)
	if !strings.HasPrefix(base, "0004_next_") {
		t.Errorf("filename = %q, want prefix 0004_next_", base)
	}
}

func TestMakeMigrationsDestroyCalledOnError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "", fmt.Errorf("fail")
	}

	destroyed := false
	destroyContainer = func(_ string) { destroyed = true }

	_, _ = MakeMigrations(context.Background(), Postgres17(t.TempDir(), "", testTables))
	if !destroyed {
		t.Fatal("expected destroyContainer to be called")
	}
}

func TestMakeMigrationsWithOnConnectSuccess(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	openDatabase = stubOpenDatabase
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "CREATE TABLE t (id INT);", nil
	}

	called := false
	cfg := Postgres17(t.TempDir(), "", testTables)
	cfg.OnConnect = func(_ context.Context, db *sql.DB) error {
		called = true
		return nil
	}

	_, err := MakeMigrations(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("OnConnect was not called")
	}
}

func TestMigrationFilename(t *testing.T) {
	tests := []struct {
		seq  int
		name string
		want string
	}{
		{1, "", "0001_"},
		{1, "add_users", "0001_add_users_"},
		{42, "init", "0042_init_"},
		{999, "", "0999_"},
	}

	for _, tt := range tests {
		got := migrationFilename(tt.seq, tt.name)
		if got[:len(tt.want)] != tt.want {
			t.Errorf("migrationFilename(%d, %q) = %q, want prefix %q", tt.seq, tt.name, got, tt.want)
		}
		if filepath.Ext(got) != ".sql" {
			t.Errorf("migrationFilename(%d, %q) = %q, want .sql extension", tt.seq, tt.name, got)
		}
	}
}

func TestNextSequence(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		seq, err := nextSequence(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seq != 1 {
			t.Errorf("seq = %d, want 1", seq)
		}
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		seq, err := nextSequence(filepath.Join(t.TempDir(), "nope"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seq != 1 {
			t.Errorf("seq = %d, want 1", seq)
		}
	})

	t.Run("existing migrations", func(t *testing.T) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "0001_20260101000000.sql"), []byte("x"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "0002_add_users_20260102000000.sql"), []byte("x"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "0005_init_20260103000000.sql"), []byte("x"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o644)

		seq, err := nextSequence(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seq != 6 {
			t.Errorf("seq = %d, want 6", seq)
		}
	})
}

func TestDefaultAllocatePort(t *testing.T) {
	port, err := defaultAllocatePort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port == "" || port == "0" {
		t.Errorf("got invalid port %q", port)
	}
}

func TestDefaultPingDatabaseTimeout(t *testing.T) {
	ctx := context.Background()
	err := defaultPingDatabase(ctx, "nonexistent_driver", "invalid://dsn", 1*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestDefaultPingDatabaseCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := defaultPingDatabase(ctx, "nonexistent_driver", "invalid://dsn", 60*time.Second)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got: %v", err)
	}
}

func TestDefaultPingDatabaseCanceledDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay so ping fails, then ctx.Done fires in select.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	err := defaultPingDatabase(ctx, "entx_stub_fail_ping", "", 60*time.Second)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got: %v", err)
	}
}

func TestDefaultRunContainerError(t *testing.T) {
	err := defaultRunContainer(context.Background(), "test-fail", "nonexistent:image", "5432", "99999", nil)
	if err == nil {
		t.Fatal("expected error for invalid image")
	}
	if !strings.Contains(err.Error(), "entx: start container") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultDestroyContainer(t *testing.T) {
	// Should not panic even for nonexistent containers.
	defaultDestroyContainer("entx-nonexistent-container-test")
}

func TestDefaultOpenDatabase(t *testing.T) {
	_, err := defaultOpenDatabase("nonexistent_driver", "invalid://dsn")
	if err == nil {
		t.Fatal("expected error for nonexistent driver")
	}
}

func TestDefaultOpenDatabaseSuccess(t *testing.T) {
	db, err := defaultOpenDatabase("entx_stub", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	db.Close()
}

func TestDefaultGenerateSchemaOpenError(t *testing.T) {
	_, err := defaultGenerateSchema(context.Background(), "nonexistent_driver", "invalid://dsn", testTables)
	if err == nil {
		t.Fatal("expected error for nonexistent driver")
	}
}

func TestDefaultGenerateSchemaCreateError(t *testing.T) {
	// entx_stub_fail_ping driver can Open but its conn fails on Prepare,
	// which causes migrate.Create to fail.
	_, err := defaultGenerateSchema(context.Background(), "entx_stub_fail_ping", "", testTables)
	if err == nil {
		t.Fatal("expected error from migrate.Create")
	}
	if !strings.Contains(err.Error(), "entx: generate schema") && !strings.Contains(err.Error(), "entx: create migrator") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNextSequenceReadDirError(t *testing.T) {
	dir := t.TempDir()
	// Remove read permission to trigger a ReadDir error that isn't NotExist.
	_ = os.Chmod(dir, 0o000)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	_, err := nextSequence(dir)
	if err == nil || !strings.Contains(err.Error(), "read migrations dir") {
		t.Fatalf("expected read dir error, got: %v", err)
	}
}

func TestMakeMigrationsNextSequenceError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "CREATE TABLE t (id INT);", nil
	}

	dir := t.TempDir()
	// MkdirAll will succeed, but then remove read permission so nextSequence's
	// ReadDir fails with a permission error (not NotExist).
	_ = os.Chmod(dir, 0o000)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	cfg := Postgres17(dir, "", testTables)
	_, err := MakeMigrations(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "read migrations dir") {
		t.Fatalf("expected next sequence error, got: %v", err)
	}
}

func TestMakeMigrationsWriteFileError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "CREATE TABLE t (id INT);", nil
	}

	dir := t.TempDir()
	// Make dir read-only so WriteFile fails (but MkdirAll and ReadDir succeed).
	_ = os.Chmod(dir, 0o555)
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	cfg := Postgres17(dir, "", testTables)
	_, err := MakeMigrations(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "write migration file") {
		t.Fatalf("expected write file error, got: %v", err)
	}
}

func TestMakeMigrationsMkdirError(t *testing.T) {
	stubFuncs(t)
	allocatePort = func() (string, error) { return "12345", nil }
	runContainer = func(_ context.Context, _, _, _, _ string, _ []string) error { return nil }
	destroyContainer = func(_ string) {}
	pingDatabase = func(_ context.Context, _, _ string, _ time.Duration) error { return nil }
	generateSchema = func(_ context.Context, _, _ string, _ []*schema.Table) (string, error) {
		return "CREATE TABLE t (id INT);", nil
	}

	// Use a path that can't be created (file as parent).
	tmpFile := filepath.Join(t.TempDir(), "blockfile")
	_ = os.WriteFile(tmpFile, []byte("x"), 0o644)
	impossibleDir := filepath.Join(tmpFile, "subdir")

	cfg := Postgres17(impossibleDir, "", testTables)
	_, err := MakeMigrations(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "create migrations dir") {
		t.Fatalf("expected mkdir error, got: %v", err)
	}
}
