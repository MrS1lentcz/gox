package entx

import "testing"

func TestMySQL84Config(t *testing.T) {
	cfg := MySQL84("/tmp/migrations", "init", testTables)

	if cfg.Image != "mysql:8.4" {
		t.Errorf("Image = %q, want mysql:8.4", cfg.Image)
	}
	testMySQLConfig(t, cfg)
}

func TestMySQL9Config(t *testing.T) {
	cfg := MySQL9("/tmp/migrations", "init", testTables)

	if cfg.Image != "mysql:9" {
		t.Errorf("Image = %q, want mysql:9", cfg.Image)
	}
	testMySQLConfig(t, cfg)
}

func testMySQLConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.Driver != "mysql" {
		t.Errorf("Driver = %q, want mysql", cfg.Driver)
	}
	if cfg.Dir != "/tmp/migrations" {
		t.Errorf("Dir = %q, want /tmp/migrations", cfg.Dir)
	}
	if cfg.Setup == nil {
		t.Fatal("Setup is nil")
	}

	env, port, dsn := cfg.Setup("33060")
	if port != "3306" {
		t.Errorf("port = %q, want 3306", port)
	}
	if want := "root:entx@tcp(localhost:33060)/entx?parseTime=true"; dsn != want {
		t.Errorf("dsn = %q, want %q", dsn, want)
	}
	hasRootPass := false
	hasDB := false
	for _, e := range env {
		if e == "MYSQL_ROOT_PASSWORD=entx" {
			hasRootPass = true
		}
		if e == "MYSQL_DATABASE=entx" {
			hasDB = true
		}
	}
	if !hasRootPass {
		t.Error("missing MYSQL_ROOT_PASSWORD")
	}
	if !hasDB {
		t.Error("missing MYSQL_DATABASE")
	}
}
