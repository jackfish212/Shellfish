// Package dbfs provides a database-backed filesystem that implements
// the mount interfaces defined in github.com/jackfish212/grasp/types.
//
// Multiple database backends are supported through the [Dialect] interface.
// Built-in dialects are provided for SQLite and PostgreSQL.
//
//	fs, err := dbfs.Open("sqlite", "data.db", types.PermRW)
//	defer fs.Close()
//	fs.Write(ctx, "hello.txt", strings.NewReader("world"))
package dbfs

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackfish212/grasp/types"
)

var (
	_ types.Provider          = (*FS)(nil)
	_ types.Readable          = (*FS)(nil)
	_ types.Writable          = (*FS)(nil)
	_ types.Mutable           = (*FS)(nil)
	_ types.MountInfoProvider = (*FS)(nil)
)

// ErrBadTable indicates an invalid table name was provided.
var ErrBadTable = errors.New("dbfs: invalid table name")

// Dialect abstracts database-specific SQL syntax.
// Implement this interface to add support for a new database backend.
type Dialect interface {
	SchemaSQL(table string) []string
	Migrate(db *sql.DB, table string) error
	Rebind(query string) string
}

// Option configures filesystem behavior.
type Option func(*config)

type config struct {
	tableName string
}

// Table sets the database table name (default "files").
func Table(name string) Option { return func(c *config) { c.tableName = name } }

// FS is a database-backed virtual filesystem implementing
// [types.Provider], [types.Readable], [types.Writable] and [types.Mutable].
type FS struct {
	db      *sql.DB
	dialect Dialect
	table   string
	dsn     string
	perm    types.Perm
	ownDB   bool
}

var (
	dialectsMu sync.RWMutex
	dialects   = map[string]Dialect{
		"sqlite":   SQLiteDialect{},
		"sqlite3":  SQLiteDialect{},
		"postgres": PostgresDialect{},
		"pgx":      PostgresDialect{},
	}
	validTable = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// Register adds or replaces a [Dialect] for the given driver name.
func Register(driver string, d Dialect) {
	dialectsMu.Lock()
	dialects[driver] = d
	dialectsMu.Unlock()
}

// Open creates a new database-backed filesystem.
//
// Supported built-in drivers: "sqlite", "sqlite3", "postgres", "pgx".
// The caller must blank-import the appropriate database/sql driver.
func Open(driver, dsn string, perm types.Perm, opts ...Option) (*FS, error) {
	d, err := lookupDialect(driver)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("dbfs: open: %w", err)
	}
	fs, err := newFS(db, d, perm, dsn, true, opts...)
	if err != nil {
		db.Close()
		return nil, err
	}
	return fs, nil
}

// OpenDB creates a filesystem from an existing [*sql.DB] connection.
// The caller remains responsible for closing db.
func OpenDB(db *sql.DB, driver string, perm types.Perm, opts ...Option) (*FS, error) {
	d, err := lookupDialect(driver)
	if err != nil {
		return nil, err
	}
	return newFS(db, d, perm, "", false, opts...)
}

func lookupDialect(driver string) (Dialect, error) {
	dialectsMu.RLock()
	d, ok := dialects[driver]
	dialectsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("dbfs: unknown driver %q; use Register to add custom dialects", driver)
	}
	return d, nil
}

func newFS(db *sql.DB, dialect Dialect, perm types.Perm, dsn string, ownDB bool, opts ...Option) (*FS, error) {
	cfg := config{tableName: "files"}
	for _, o := range opts {
		o(&cfg)
	}
	if !validTable.MatchString(cfg.tableName) {
		return nil, fmt.Errorf("%w: %q", ErrBadTable, cfg.tableName)
	}
	fs := &FS{db: db, dialect: dialect, table: cfg.tableName, dsn: dsn, perm: perm, ownDB: ownDB}
	for _, stmt := range dialect.SchemaSQL(cfg.tableName) {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("dbfs: schema: %w", err)
		}
	}
	if err := dialect.Migrate(db, cfg.tableName); err != nil {
		return nil, fmt.Errorf("dbfs: migrate: %w", err)
	}
	return fs, nil
}

// Close closes the database connection if it was created by [Open].
func (fs *FS) Close() error {
	if fs.ownDB {
		return fs.db.Close()
	}
	return nil
}

// DB returns the underlying [*sql.DB] for advanced usage.
func (fs *FS) DB() *sql.DB { return fs.db }

// MountInfo implements [types.MountInfoProvider].
func (fs *FS) MountInfo() (string, string) { return "dbfs", fs.dsn }

// ──── types.Provider ────

func (fs *FS) Stat(_ context.Context, path string) (*types.Entry, error) {
	path = normPath(path)

	var entry types.Entry
	var permInt int
	var modified int64
	var version int64
	var isDir bool
	var metaStr sql.NullString

	err := fs.db.QueryRow(
		fs.q(`SELECT path, is_dir, perm, modified, version, meta FROM {t} WHERE path = ?`), path,
	).Scan(&entry.Path, &isDir, &permInt, &modified, &version, &metaStr)

	if err == sql.ErrNoRows {
		like := path + "/%"
		if path == "" {
			like = "%"
		}
		var n int
		if e := fs.db.QueryRow(fs.q(`SELECT COUNT(*) FROM {t} WHERE path LIKE ?`), like).Scan(&n); e == nil && n > 0 {
			return &types.Entry{Name: baseName(path), Path: path, IsDir: true, Perm: types.PermRX}, nil
		}
		if path == "" {
			return &types.Entry{Name: "/", Path: "", IsDir: true, Perm: types.PermRX}, nil
		}
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	if err != nil {
		return nil, fmt.Errorf("dbfs: stat: %w", err)
	}

	entry.Name = baseName(path)
	entry.IsDir = isDir
	entry.Perm = types.Perm(permInt)
	entry.Modified = time.Unix(modified, 0)
	entry.Meta = decodeMeta(metaStr)
	if entry.Meta == nil {
		entry.Meta = make(map[string]string)
	}
	entry.Meta["version"] = strconv.FormatInt(version, 10)

	if !isDir {
		if err := fs.db.QueryRow(fs.q(`SELECT LENGTH(content) FROM {t} WHERE path = ?`), path).Scan(&entry.Size); err != nil {
			return nil, fmt.Errorf("dbfs: stat: %w", err)
		}
	}
	return &entry, nil
}

func (fs *FS) List(_ context.Context, path string, _ types.ListOpts) ([]types.Entry, error) {
	path = normPath(path)

	var rows *sql.Rows
	var err error
	if path == "" {
		rows, err = fs.db.Query(fs.q(`SELECT path FROM {t} ORDER BY path`))
	} else {
		rows, err = fs.db.Query(fs.q(`SELECT path FROM {t} WHERE path LIKE ? ORDER BY path`), path+"/%")
	}
	if err != nil {
		return nil, fmt.Errorf("dbfs: list: %w", err)
	}
	defer rows.Close()

	pfx := path + "/"
	if path == "" {
		pfx = ""
	}
	seen := make(map[string]bool)
	var entries []types.Entry
	hasRows := false

	for rows.Next() {
		hasRows = true
		var cp string
		if err := rows.Scan(&cp); err != nil {
			return nil, err
		}
		if cp == path {
			continue
		}

		rest := cp
		if pfx != "" {
			if !strings.HasPrefix(cp, pfx) {
				continue
			}
			rest = cp[len(pfx):]
		}
		if rest == "" {
			continue
		}

		name := rest
		implicit := false
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name = rest[:i]
			implicit = true
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		full := pfx + name
		if implicit {
			entries = append(entries, types.Entry{Name: name, Path: full, IsDir: true, Perm: types.PermRX})
		} else if e, err := fs.Stat(context.Background(), full); err == nil {
			entries = append(entries, *e)
		}
	}

	if path != "" && !hasRows {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	return entries, nil
}

// ──── types.Readable ────

func (fs *FS) Open(_ context.Context, path string) (types.File, error) {
	path = normPath(path)

	var content []byte
	var isDir bool
	var permInt int
	var modified, version int64
	var metaStr sql.NullString

	err := fs.db.QueryRow(
		fs.q(`SELECT content, is_dir, perm, modified, version, meta FROM {t} WHERE path = ?`), path,
	).Scan(&content, &isDir, &permInt, &modified, &version, &metaStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	if err != nil {
		return nil, fmt.Errorf("dbfs: open: %w", err)
	}

	perm := types.Perm(permInt)
	if !perm.CanRead() {
		return nil, fmt.Errorf("%w: %s", types.ErrNotReadable, path)
	}

	meta := decodeMeta(metaStr)
	if meta == nil {
		meta = make(map[string]string)
	}
	meta["version"] = strconv.FormatInt(version, 10)

	entry := &types.Entry{
		Name: baseName(path), Path: path, IsDir: isDir,
		Perm: perm, Size: int64(len(content)),
		Modified: time.Unix(modified, 0), Meta: meta,
	}
	return types.NewFile(path, entry, io.NopCloser(bytes.NewReader(content))), nil
}

// ──── types.Writable ────

func (fs *FS) Write(_ context.Context, path string, r io.Reader) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("dbfs: read content: %w", err)
	}
	path = normPath(path)
	_, err = fs.db.Exec(fs.q(`
		INSERT INTO {t} (path, content, is_dir, perm, modified, version) VALUES (?, ?, ?, ?, ?, 1)
		ON CONFLICT(path) DO UPDATE SET content=excluded.content, is_dir=excluded.is_dir,
			perm=excluded.perm, modified=excluded.modified, version={t}.version+1
	`), path, data, false, int(fs.perm), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("dbfs: write: %w", err)
	}
	return nil
}

// ──── types.Mutable ────

func (fs *FS) Mkdir(_ context.Context, path string, perm types.Perm) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	path = normPath(path)
	_, err := fs.db.Exec(
		fs.q(`INSERT INTO {t} (path, content, is_dir, perm, modified) VALUES (?, NULL, ?, ?, ?) ON CONFLICT(path) DO NOTHING`),
		path, true, int(perm), time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("dbfs: mkdir: %w", err)
	}
	return nil
}

func (fs *FS) Remove(_ context.Context, path string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	path = normPath(path)

	var exists bool
	if err := fs.db.QueryRow(fs.q(`SELECT EXISTS(SELECT 1 FROM {t} WHERE path = ?)`), path).Scan(&exists); err != nil {
		return fmt.Errorf("dbfs: remove: %w", err)
	}
	if !exists {
		var n int
		if err := fs.db.QueryRow(fs.q(`SELECT COUNT(*) FROM {t} WHERE path LIKE ?`), path+"/%").Scan(&n); err != nil {
			return fmt.Errorf("dbfs: remove: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("%w: %s", types.ErrNotFound, path)
		}
	}

	_, err := fs.db.Exec(fs.q(`DELETE FROM {t} WHERE path = ? OR path LIKE ?`), path, path+"/%")
	return err
}

func (fs *FS) Rename(_ context.Context, oldPath, newPath string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, oldPath)
	}
	oldPath = normPath(oldPath)
	newPath = normPath(newPath)

	var exists bool
	if err := fs.db.QueryRow(fs.q(`SELECT EXISTS(SELECT 1 FROM {t} WHERE path = ?)`), oldPath).Scan(&exists); err != nil || !exists {
		return fmt.Errorf("%w: %s", types.ErrNotFound, oldPath)
	}

	tx, err := fs.db.Begin()
	if err != nil {
		return fmt.Errorf("dbfs: rename: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Unix()
	if _, err := tx.Exec(fs.q(`UPDATE {t} SET path = ?, modified = ? WHERE path = ?`), newPath, now, oldPath); err != nil {
		return fmt.Errorf("dbfs: rename: %w", err)
	}

	oldPfx := oldPath + "/"
	newPfx := newPath + "/"
	if _, err := tx.Exec(
		fs.q(`UPDATE {t} SET path = ? || SUBSTR(path, ?), modified = ? WHERE path LIKE ?`),
		newPfx, len(oldPfx)+1, now, oldPfx+"%",
	); err != nil {
		return fmt.Errorf("dbfs: rename children: %w", err)
	}

	return tx.Commit()
}

// ──── Extended API ────

// WriteFile writes content with metadata in a single operation.
// The version column is automatically incremented on each write.
func (fs *FS) WriteFile(_ context.Context, path string, content []byte, meta map[string]string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	path = normPath(path)
	_, err := fs.db.Exec(fs.q(`
		INSERT INTO {t} (path, content, is_dir, perm, modified, version, meta) VALUES (?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(path) DO UPDATE SET content=excluded.content, is_dir=excluded.is_dir,
			perm=excluded.perm, modified=excluded.modified, version={t}.version+1, meta=excluded.meta
	`), path, content, false, int(fs.perm), time.Now().Unix(), encodeMeta(meta))
	if err != nil {
		return fmt.Errorf("dbfs: write file: %w", err)
	}
	return nil
}

// WriteMeta updates only the metadata without touching content or version.
func (fs *FS) WriteMeta(_ context.Context, path string, meta map[string]string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	path = normPath(path)
	res, err := fs.db.Exec(fs.q(`UPDATE {t} SET meta = ? WHERE path = ?`), encodeMeta(meta), path)
	if err != nil {
		return fmt.Errorf("dbfs: write meta: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	return nil
}

// Purge deletes non-directory files older than the given duration.
func (fs *FS) Purge(_ context.Context, olderThan time.Duration) (int64, error) {
	res, err := fs.db.Exec(
		fs.q(`DELETE FROM {t} WHERE NOT is_dir AND modified < ?`),
		time.Now().Add(-olderThan).Unix(),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PurgeByPrefix deletes all entries under a path prefix.
func (fs *FS) PurgeByPrefix(_ context.Context, prefix string) (int64, error) {
	prefix = normPath(prefix)
	res, err := fs.db.Exec(fs.q(`DELETE FROM {t} WHERE path = ? OR path LIKE ?`), prefix, prefix+"/%")
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TotalSize returns the sum of content sizes for all non-directory files.
func (fs *FS) TotalSize(_ context.Context) (int64, error) {
	var sz sql.NullInt64
	if err := fs.db.QueryRow(fs.q(`SELECT SUM(LENGTH(content)) FROM {t} WHERE NOT is_dir`)).Scan(&sz); err != nil {
		return 0, err
	}
	return sz.Int64, nil
}

// Count returns the number of non-directory files.
func (fs *FS) Count(_ context.Context) (int64, error) {
	var n int64
	err := fs.db.QueryRow(fs.q(`SELECT COUNT(*) FROM {t} WHERE NOT is_dir`)).Scan(&n)
	return n, err
}

// ──── internal helpers ────

func (fs *FS) q(query string) string {
	return fs.dialect.Rebind(strings.ReplaceAll(query, "{t}", fs.table))
}

func normPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	return strings.TrimSuffix(p, "/")
}

func baseName(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func encodeMeta(m map[string]string) sql.NullString {
	if len(m) == 0 {
		return sql.NullString{}
	}
	data, _ := json.Marshal(m)
	return sql.NullString{String: string(data), Valid: true}
}

func decodeMeta(s sql.NullString) map[string]string {
	if !s.Valid || s.String == "" {
		return nil
	}
	var m map[string]string
	if json.Unmarshal([]byte(s.String), &m) != nil {
		return nil
	}
	return m
}
