package entx

import "testing"

func TestPostgres17Config(t *testing.T) {
	cfg := Postgres17("/tmp/migrations", "init", testTables)

	if cfg.Image != "postgres:17-alpine" {
		t.Errorf("Image = %q, want postgres:17-alpine", cfg.Image)
	}
	testPostgresConfig(t, cfg)
}

func TestPostgres18Config(t *testing.T) {
	cfg := Postgres18("/tmp/migrations", "init", testTables)

	if cfg.Image != "postgres:18-alpine" {
		t.Errorf("Image = %q, want postgres:18-alpine", cfg.Image)
	}
	testPostgresConfig(t, cfg)
}

func testPostgresConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.Driver != "postgres" {
		t.Errorf("Driver = %q, want postgres", cfg.Driver)
	}
	if cfg.Dir != "/tmp/migrations" {
		t.Errorf("Dir = %q, want /tmp/migrations", cfg.Dir)
	}
	if cfg.Name != "init" {
		t.Errorf("Name = %q, want init", cfg.Name)
	}
	if cfg.Setup == nil {
		t.Fatal("Setup is nil")
	}

	env, port, dsn := cfg.Setup("12345")
	if port != "5432" {
		t.Errorf("port = %q, want 5432", port)
	}
	if want := "postgres://entx:entx@localhost:12345/entx?sslmode=disable"; dsn != want {
		t.Errorf("dsn = %q, want %q", dsn, want)
	}
	wantEnv := map[string]bool{
		"POSTGRES_USER=entx":     true,
		"POSTGRES_PASSWORD=entx": true,
		"POSTGRES_DB=entx":       true,
	}
	for _, e := range env {
		if !wantEnv[e] {
			t.Errorf("unexpected env var: %q", e)
		}
		delete(wantEnv, e)
	}
	for e := range wantEnv {
		t.Errorf("missing env var: %q", e)
	}
}
