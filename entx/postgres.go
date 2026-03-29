package entx

import (
	"fmt"

	"entgo.io/ent/dialect/sql/schema"
)

// Postgres17 returns a Config for generating migrations against PostgreSQL 17 Alpine.
func Postgres17(dir, name string, tables []*schema.Table) Config {
	return Postgres("17", dir, name, tables)
}

// Postgres18 returns a Config for generating migrations against PostgreSQL 18 Alpine.
func Postgres18(dir, name string, tables []*schema.Table) Config {
	return Postgres("18", dir, name, tables)
}

// Postgres returns a Config for generating migrations against a PostgreSQL
// Alpine container. The version is the major Postgres version (e.g. "16", "17").
func Postgres(version, dir, name string, tables []*schema.Table) Config {
	const (
		user   = "entx"
		pass   = "entx"
		dbname = "entx"
	)

	return Config{
		Driver: "postgres",
		Image:  "postgres:" + version + "-alpine",
		Dir:    dir,
		Name:   name,
		Tables: tables,
		Setup: func(hostPort string) ([]Env, ContainerPort, DSN) {
			return []Env{
					"POSTGRES_USER=" + user,
					"POSTGRES_PASSWORD=" + pass,
					"POSTGRES_DB=" + dbname,
				},
				"5432",
				DSN(fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", user, pass, hostPort, dbname))
		},
	}
}
