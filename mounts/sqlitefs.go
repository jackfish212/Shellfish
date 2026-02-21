package mounts

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackfish212/grasp/types"

	_ "modernc.org/sqlite"
)

var (
	_ types.Provider = (*SQLiteFS)(nil)
	_ types.Readable = (*SQLiteFS)(nil)
	_ types.Writable = (*SQLiteFS)(nil)
	_ types.Mutable  = (*SQLiteFS)(nil)
)

// SQLiteFS is a SQLite-backed filesystem.
type SQLiteFS struct {
	db     *sql.DB
	dbPath string
	perm   types.Perm
	mu     sync.RWMutex
}

func NewSQLiteFS(dbPath string, perm types.Perm) (*SQLiteFS, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}
	fs := &SQLiteFS{db: db, dbPath: dbPath, perm: perm}
	if err := fs.initDB(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing database: %w", err)
	}
	return fs, nil
}

func (fs *SQLiteFS) initDB() error {
	schema := `
	CREATE TABLE IF NOT EXISTS files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE NOT NULL,
		content BLOB,
		is_dir BOOLEAN NOT NULL DEFAULT 0,
		perm INTEGER NOT NULL DEFAULT 1,
		modified INTEGER NOT NULL DEFAULT 0,
		meta TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
	`
	_, err := fs.db.Exec(schema)
	return err
}

func (fs *SQLiteFS) Close() error { return fs.db.Close() }

func (fs *SQLiteFS) Stat(_ context.Context, path string) (*types.Entry, error) {
	path = normPath(path)

	var entry types.Entry
	var permInt int
	var modified int64
	var isDir bool

	err := fs.db.QueryRow(`SELECT path, is_dir, perm, modified FROM files WHERE path = ?`, path).Scan(&entry.Path, &isDir, &permInt, &modified)

	if err == sql.ErrNoRows {
		prefix := path + "/"
		if path == "" {
			prefix = ""
		}
		var count int
		checkPath := prefix + "%"
		if prefix == "" {
			checkPath = "%"
		}
		err = fs.db.QueryRow(`SELECT COUNT(*) FROM files WHERE path LIKE ?`, checkPath).Scan(&count)
		if err == nil && count > 0 {
			return &types.Entry{Name: baseName(path), Path: path, IsDir: true, Perm: types.PermRX}, nil
		}
		if path == "" {
			return &types.Entry{Name: "/", Path: "", IsDir: true, Perm: types.PermRX}, nil
		}
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	if err != nil {
		return nil, fmt.Errorf("stat error: %w", err)
	}

	entry.Name = baseName(path)
	entry.IsDir = isDir
	entry.Perm = types.Perm(permInt)
	entry.Modified = time.Unix(modified, 0)

	if !isDir {
		fs.db.QueryRow(`SELECT LENGTH(content) FROM files WHERE path = ?`, path).Scan(&entry.Size)
	}

	return &entry, nil
}

func (fs *SQLiteFS) List(_ context.Context, path string, _ types.ListOpts) ([]types.Entry, error) {
	path = normPath(path)

	var rows *sql.Rows
	var err error

	if path == "" {
		rows, err = fs.db.Query(`SELECT path FROM files ORDER BY path`)
	} else {
		prefix := path + "/"
		rows, err = fs.db.Query(`SELECT path FROM files WHERE path LIKE ? || '%' ORDER BY path`, prefix)
	}
	if err != nil {
		return nil, fmt.Errorf("list error: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	var entries []types.Entry

	prefix := path + "/"
	if path == "" {
		prefix = ""
	}

	hasRows := false
	for rows.Next() {
		hasRows = true
		var childPath string
		if err := rows.Scan(&childPath); err != nil {
			return nil, fmt.Errorf("scanning path: %w", err)
		}
		if childPath == path {
			continue
		}
		rest := childPath
		if prefix != "" {
			if !strings.HasPrefix(childPath, prefix) {
				continue
			}
			rest = strings.TrimPrefix(childPath, prefix)
		}
		if rest == "" || rest == childPath && prefix != "" {
			continue
		}

		name := rest
		isImplicitDir := false
		if idx := strings.IndexByte(rest, '/'); idx >= 0 {
			name = rest[:idx]
			isImplicitDir = true
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		childFullPath := prefix + name
		if isImplicitDir {
			entries = append(entries, types.Entry{Name: name, Path: childFullPath, IsDir: true, Perm: types.PermRX})
		} else {
			entry, err := fs.Stat(context.Background(), childFullPath)
			if err != nil {
				continue
			}
			entries = append(entries, *entry)
		}
	}

	if path != "" && !hasRows {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	return entries, nil
}

func (fs *SQLiteFS) Open(_ context.Context, path string) (types.File, error) {
	path = normPath(path)
	var content []byte
	var isDir bool
	var permInt int
	var modified int64

	err := fs.db.QueryRow(`SELECT content, is_dir, perm, modified FROM files WHERE path = ?`, path).Scan(&content, &isDir, &permInt, &modified)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	if err != nil {
		return nil, fmt.Errorf("open error: %w", err)
	}

	perm := types.Perm(permInt)
	if !perm.CanRead() {
		return nil, fmt.Errorf("%w: %s", types.ErrNotReadable, path)
	}

	entry := &types.Entry{Name: baseName(path), Path: path, IsDir: isDir, Perm: perm, Size: int64(len(content)), Modified: time.Unix(modified, 0)}
	br := newBytesReader(content)
	return types.NewFile(path, entry, io.NopCloser(br)), nil
}

type bytesReader struct {
	b []byte
	i int64
}

func newBytesReader(b []byte) *bytesReader { return &bytesReader{b: b} }

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.i >= int64(len(r.b)) {
		return 0, io.EOF
	}
	n = copy(p, r.b[r.i:])
	r.i += int64(n)
	return n, nil
}

func (fs *SQLiteFS) Write(_ context.Context, path string, r io.Reader) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	content, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading content: %w", err)
	}
	path = normPath(path)
	parent := filepath.Dir(path)
	if parent != "." && parent != "" {
		fs.ensureDir(parent)
	}

	_, err = fs.db.Exec(`
		INSERT INTO files (path, content, is_dir, perm, modified) VALUES (?, ?, 0, ?, ?)
		ON CONFLICT(path) DO UPDATE SET content = excluded.content, is_dir = excluded.is_dir, perm = excluded.perm, modified = excluded.modified
	`, path, content, int(fs.perm), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

func (fs *SQLiteFS) ensureDir(path string) error { return nil }

func (fs *SQLiteFS) Mkdir(_ context.Context, path string, perm types.Perm) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	path = normPath(path)
	_, err := fs.db.Exec(`INSERT OR IGNORE INTO files (path, content, is_dir, perm, modified) VALUES (?, NULL, 1, ?, ?)`, path, int(perm), time.Now().Unix())
	if err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return nil
}

func (fs *SQLiteFS) Remove(_ context.Context, path string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	path = normPath(path)

	var exists bool
	fs.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM files WHERE path = ?)`, path).Scan(&exists)
	if !exists {
		prefix := path + "/"
		var count int
		fs.db.QueryRow(`SELECT COUNT(*) FROM files WHERE path LIKE ?`, prefix+"%").Scan(&count)
		if count == 0 {
			return fmt.Errorf("%w: %s", types.ErrNotFound, path)
		}
	}

	_, err := fs.db.Exec(`DELETE FROM files WHERE path = ? OR path LIKE ? || '%'`, path, path+"/")
	return err
}

func (fs *SQLiteFS) Rename(_ context.Context, oldPath, newPath string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, oldPath)
	}
	oldPath = normPath(oldPath)
	newPath = normPath(newPath)

	var exists bool
	if err := fs.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM files WHERE path = ?)`, oldPath).Scan(&exists); err != nil || !exists {
		return fmt.Errorf("%w: %s", types.ErrNotFound, oldPath)
	}

	tx, err := fs.db.Begin()
	if err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE files SET path = ?, modified = ? WHERE path = ?`, newPath, time.Now().Unix(), oldPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	oldPrefix := oldPath + "/"
	newPrefix := newPath + "/"
	if _, err := tx.Exec(`UPDATE files SET path = ? || SUBSTR(path, ?), modified = ? WHERE path LIKE ? || '%'`, newPrefix, len(oldPrefix)+1, time.Now().Unix(), oldPrefix); err != nil {
		return fmt.Errorf("rename children: %w", err)
	}

	return tx.Commit()
}

func (fs *SQLiteFS) MountInfo() (string, string) { return "sqlitefs", fs.dbPath }
