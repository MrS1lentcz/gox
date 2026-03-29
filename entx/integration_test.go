package entx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"entgo.io/ent/dialect/sql/schema"
	"entgo.io/ent/schema/field"
)

func TestMakeMigrationsIntegrationPostgres(t *testing.T) {

	tables := []*schema.Table{
		{
			Name: "test_users",
			Columns: []*schema.Column{
				{Name: "id", Type: field.TypeInt, Increment: true},
				{Name: "name", Type: field.TypeString},
				{Name: "email", Type: field.TypeString, Unique: true},
				{Name: "created_at", Type: field.TypeTime},
			},
		},
	}
	tables[0].PrimaryKey = []*schema.Column{tables[0].Columns[0]}

	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	path, err := MakeMigrations(ctx, Postgres17(dir, "init", tables))
	if err != nil {
		t.Fatalf("MakeMigrations failed: %v", err)
	}

	if path == "" {
		t.Fatal("expected non-empty path")
	}

	base := filepath.Base(path)
	if !strings.HasPrefix(base, "0001_init_") {
		t.Errorf("filename = %q, want prefix 0001_init_", base)
	}
	if !strings.HasSuffix(base, ".sql") {
		t.Errorf("filename = %q, want .sql suffix", base)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration file: %v", err)
	}

	sql := string(content)
	if !strings.Contains(sql, "test_users") {
		t.Errorf("migration SQL should contain 'test_users', got:\n%s", sql)
	}
	if !strings.Contains(sql, "CREATE TABLE") {
		t.Errorf("migration SQL should contain 'CREATE TABLE', got:\n%s", sql)
	}

	// Run a second time to verify sequence increments.
	path2, err := MakeMigrations(ctx, Postgres17(dir, "second", tables))
	if err != nil {
		t.Fatalf("second MakeMigrations failed: %v", err)
	}
	if path2 != "" {
		base2 := filepath.Base(path2)
		if !strings.HasPrefix(base2, "0002_second_") {
			t.Errorf("second filename = %q, want prefix 0002_second_", base2)
		}
	}
}
