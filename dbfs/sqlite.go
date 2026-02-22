package dbfs

import (
	"database/sql"
	"fmt"
)

// SQLiteDialect implements [Dialect] for SQLite databases.
//
// Compatible drivers: modernc.org/sqlite ("sqlite"), github.com/mattn/go-sqlite3 ("sqlite3").
type SQLiteDialect struct{}

func (SQLiteDialect) SchemaSQL(table string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			path     TEXT UNIQUE NOT NULL,
			content  BLOB,
			is_dir   INTEGER NOT NULL DEFAULT 0,
			perm     INTEGER NOT NULL DEFAULT 1,
			modified INTEGER NOT NULL DEFAULT 0,
			version  INTEGER NOT NULL DEFAULT 1,
			meta     TEXT
		)`, table),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_path ON %s(path)`, table, table),
	}
}

func (SQLiteDialect) Migrate(db *sql.DB, table string) error {
	var count int
	q := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name='version'`, table)
	if err := db.QueryRow(q).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		_, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN version INTEGER NOT NULL DEFAULT 1`, table))
		return err
	}
	return nil
}

func (SQLiteDialect) Rebind(query string) string { return query }
