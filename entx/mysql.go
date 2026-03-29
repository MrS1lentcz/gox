package entx

import (
	"fmt"

	"entgo.io/ent/dialect/sql/schema"
)

// MySQL84 returns a Config for generating migrations against MySQL 8.4 (LTS).
func MySQL84(dir, name string, tables []*schema.Table) Config {
	return MySQL("8.4", dir, name, tables)
}

// MySQL9 returns a Config for generating migrations against MySQL 9.
func MySQL9(dir, name string, tables []*schema.Table) Config {
	return MySQL("9", dir, name, tables)
}

// MySQL returns a Config for generating migrations against a MySQL container.
// The version corresponds to the Docker image tag (e.g. "8.4", "9").
func MySQL(version, dir, name string, tables []*schema.Table) Config {
	const (
		pass   = "entx"
		dbname = "entx"
	)

	return Config{
		Driver: "mysql",
		Image:  "mysql:" + version,
		Dir:    dir,
		Name:   name,
		Tables: tables,
		Setup: func(hostPort string) ([]Env, ContainerPort, DSN) {
			return []Env{
					"MYSQL_ROOT_PASSWORD=" + pass,
					"MYSQL_DATABASE=" + dbname,
				},
				"3306",
				DSN(fmt.Sprintf("root:%s@tcp(localhost:%s)/%s?parseTime=true", pass, hostPort, dbname))
		},
	}
}
