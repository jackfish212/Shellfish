package dbfs

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// PostgresDialect implements [Dialect] for PostgreSQL databases.
//
// Compatible drivers: github.com/jackc/pgx/v5/stdlib ("pgx"), github.com/lib/pq ("postgres").
type PostgresDialect struct{}

func (PostgresDialect) SchemaSQL(table string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id       BIGSERIAL PRIMARY KEY,
			path     TEXT UNIQUE NOT NULL,
			content  BYTEA,
			is_dir   BOOLEAN  NOT NULL DEFAULT FALSE,
			perm     INTEGER  NOT NULL DEFAULT 1,
			modified BIGINT   NOT NULL DEFAULT 0,
			version  BIGINT   NOT NULL DEFAULT 1,
			meta     JSONB
		)`, table),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_path ON %s(path)`, table, table),
	}
}

func (PostgresDialect) Migrate(db *sql.DB, table string) error {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = $1 AND column_name = 'version'`,
		table,
	).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN version BIGINT NOT NULL DEFAULT 1`, table))
		return err
	}
	return nil
}

// Rebind converts ? placeholders to PostgreSQL's $1, $2, ... style.
func (PostgresDialect) Rebind(query string) string {
	var buf strings.Builder
	buf.Grow(len(query) + 16)
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			buf.WriteByte('$')
			buf.WriteString(strconv.Itoa(n))
			n++
		} else {
			buf.WriteByte(query[i])
		}
	}
	return buf.String()
}
