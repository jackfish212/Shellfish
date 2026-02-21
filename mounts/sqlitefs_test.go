package mounts

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackfish212/grasp/types"

	_ "modernc.org/sqlite"
)

func setupSQLiteFS(t *testing.T) *SQLiteFS {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	fs, err := NewSQLiteFS(dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("NewSQLiteFS: %v", err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	return fs
}

func TestSQLiteFSWriteAndOpen(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	err := fs.Write(ctx, "hello.txt", strings.NewReader("hello sqlite"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := fs.Open(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(f)
	if string(data) != "hello sqlite" {
		t.Errorf("content = %q, want %q", string(data), "hello sqlite")
	}
}

func TestSQLiteFSStat(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "file.txt", strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}

	entry, err := fs.Stat(ctx, "file.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.Name != "file.txt" {
		t.Errorf("Name = %q", entry.Name)
	}
	if entry.IsDir {
		t.Error("should not be dir")
	}
	if entry.Size != 4 {
		t.Errorf("Size = %d, want 4", entry.Size)
	}
}

func TestSQLiteFSStatImplicitDir(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "docs/readme.md", strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}

	entry, err := fs.Stat(ctx, "docs")
	if err != nil {
		t.Fatalf("Stat implicit dir: %v", err)
	}
	if !entry.IsDir {
		t.Error("should be implicit directory")
	}
}

func TestSQLiteFSStatNotFound(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	_, err := fs.Stat(ctx, "ghost.txt")
	if err == nil {
		t.Error("Stat nonexistent should fail")
	}
}

func TestSQLiteFSStatRoot(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	entry, err := fs.Stat(ctx, "")
	if err != nil {
		t.Fatalf("Stat root: %v", err)
	}
	if !entry.IsDir {
		t.Error("root should be dir")
	}
}

func TestSQLiteFSList(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "a.txt", strings.NewReader("a")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Write(ctx, "b.txt", strings.NewReader("b")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Write(ctx, "sub/c.txt", strings.NewReader("c")); err != nil {
		t.Fatal(err)
	}

	entries, err := fs.List(ctx, "", types.ListOpts{})
	if err != nil {
		t.Fatalf("List root: %v", err)
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("missing files: %v", names)
	}
	if !names["sub"] {
		t.Error("missing implicit dir 'sub'")
	}
}

func TestSQLiteFSListSubdir(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "dir/file1.txt", strings.NewReader("1")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Write(ctx, "dir/file2.txt", strings.NewReader("2")); err != nil {
		t.Fatal(err)
	}

	entries, err := fs.List(ctx, "dir", types.ListOpts{})
	if err != nil {
		t.Fatalf("List dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("List dir returned %d entries, want 2", len(entries))
	}
}

func TestSQLiteFSListNotFound(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	_, err := fs.List(ctx, "nonexistent", types.ListOpts{})
	if err == nil {
		t.Error("List nonexistent should fail")
	}
}

func TestSQLiteFSOverwrite(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "file.txt", strings.NewReader("v1")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Write(ctx, "file.txt", strings.NewReader("v2")); err != nil {
		t.Fatal(err)
	}

	f, _ := fs.Open(ctx, "file.txt")
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(f)
	if string(data) != "v2" {
		t.Errorf("overwrite = %q, want v2", string(data))
	}
}

func TestSQLiteFSWriteReadOnly(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewSQLiteFS(filepath.Join(dir, "ro.db"), types.PermRO)
	if err != nil {
		t.Fatalf("NewSQLiteFS: %v", err)
	}
	defer func() { _ = fs.Close() }()

	ctx := context.Background()
	err = fs.Write(ctx, "file.txt", strings.NewReader("data"))
	if err == nil {
		t.Error("Write on RO fs should fail")
	}
}

func TestSQLiteFSMkdir(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	err := fs.Mkdir(ctx, "newdir", types.PermRWX)
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	entry, err := fs.Stat(ctx, "newdir")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !entry.IsDir {
		t.Error("should be directory")
	}
}

func TestSQLiteFSRemove(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "removeme.txt", strings.NewReader("bye")); err != nil {
		t.Fatal(err)
	}
	err := fs.Remove(ctx, "removeme.txt")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err = fs.Stat(ctx, "removeme.txt")
	if err == nil {
		t.Error("Stat after Remove should fail")
	}
}

func TestSQLiteFSRemoveRecursive(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Mkdir(ctx, "parent", types.PermRWX); err != nil {
		t.Fatal(err)
	}
	if err := fs.Write(ctx, "parent/child.txt", strings.NewReader("c")); err != nil {
		t.Fatal(err)
	}

	err := fs.Remove(ctx, "parent")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err = fs.Stat(ctx, "parent/child.txt")
	if err == nil {
		t.Error("children should be removed")
	}
}

func TestSQLiteFSRemoveNotFound(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	err := fs.Remove(ctx, "ghost.txt")
	if err == nil {
		t.Error("Remove nonexistent should fail")
	}
}

func TestSQLiteFSRename(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "old.txt", strings.NewReader("content")); err != nil {
		t.Fatal(err)
	}
	err := fs.Rename(ctx, "old.txt", "new.txt")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	_, err = fs.Stat(ctx, "old.txt")
	if err == nil {
		t.Error("old should not exist")
	}

	f, err := fs.Open(ctx, "new.txt")
	if err != nil {
		t.Fatalf("Open new.txt: %v", err)
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(f)
	if string(data) != "content" {
		t.Errorf("renamed content = %q", string(data))
	}
}

func TestSQLiteFSRenameNotFound(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	err := fs.Rename(ctx, "ghost.txt", "new.txt")
	if err == nil {
		t.Error("Rename nonexistent should fail")
	}
}

func TestSQLiteFSPersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	fs1, err := NewSQLiteFS(dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("NewSQLiteFS: %v", err)
	}
	ctx := context.Background()
	if err := fs1.Write(ctx, "persistent.txt", strings.NewReader("survive restart")); err != nil {
		t.Fatal(err)
	}
	_ = fs1.Close()

	fs2, err := NewSQLiteFS(dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("NewSQLiteFS reopen: %v", err)
	}
	defer func() { _ = fs2.Close() }()

	f, err := fs2.Open(ctx, "persistent.txt")
	if err != nil {
		t.Fatalf("Open after reopen: %v", err)
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(f)
	if string(data) != "survive restart" {
		t.Errorf("persisted content = %q", string(data))
	}
}

func TestSQLiteFSMountInfo(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "info.db")
	fs, _ := NewSQLiteFS(dbPath, types.PermRW)
	defer func() { _ = fs.Close() }()

	name, extra := fs.MountInfo()
	if name != "sqlitefs" {
		t.Errorf("MountInfo name = %q", name)
	}
	if extra != dbPath {
		t.Errorf("MountInfo extra = %q, want %q", extra, dbPath)
	}
}

func TestSQLiteFSInvalidPath(t *testing.T) {
	_, err := NewSQLiteFS(filepath.Join(os.DevNull, "impossible", "path.db"), types.PermRW)
	if err == nil {
		t.Error("NewSQLiteFS with invalid path should fail")
	}
}

func TestSQLiteFSVersion(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "f.txt", strings.NewReader("v1")); err != nil {
		t.Fatal(err)
	}
	e1, _ := fs.Stat(ctx, "f.txt")
	if e1.Meta["version"] != "1" {
		t.Errorf("version after first write = %q, want 1", e1.Meta["version"])
	}

	if err := fs.Write(ctx, "f.txt", strings.NewReader("v2")); err != nil {
		t.Fatal(err)
	}
	e2, _ := fs.Stat(ctx, "f.txt")
	if e2.Meta["version"] != "2" {
		t.Errorf("version after second write = %q, want 2", e2.Meta["version"])
	}

	if err := fs.Write(ctx, "f.txt", strings.NewReader("v3")); err != nil {
		t.Fatal(err)
	}
	e3, _ := fs.Stat(ctx, "f.txt")
	if e3.Meta["version"] != "3" {
		t.Errorf("version after third write = %q, want 3", e3.Meta["version"])
	}
}

func TestSQLiteFSWriteFileWithMeta(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	meta := map[string]string{"etag": `"abc123"`, "source": "feed"}
	if err := fs.WriteFile(ctx, "cached.txt", []byte("content"), meta); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entry, err := fs.Stat(ctx, "cached.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.Meta["etag"] != `"abc123"` {
		t.Errorf("meta etag = %q", entry.Meta["etag"])
	}
	if entry.Meta["source"] != "feed" {
		t.Errorf("meta source = %q", entry.Meta["source"])
	}
	if entry.Meta["version"] != "1" {
		t.Errorf("version = %q, want 1", entry.Meta["version"])
	}

	// Overwrite bumps version, preserves new meta
	meta2 := map[string]string{"etag": `"def456"`}
	if err := fs.WriteFile(ctx, "cached.txt", []byte("updated"), meta2); err != nil {
		t.Fatal(err)
	}
	entry2, _ := fs.Stat(ctx, "cached.txt")
	if entry2.Meta["version"] != "2" {
		t.Errorf("version after update = %q, want 2", entry2.Meta["version"])
	}
	if entry2.Meta["etag"] != `"def456"` {
		t.Errorf("updated etag = %q", entry2.Meta["etag"])
	}
}

func TestSQLiteFSWriteMeta(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "f.txt", strings.NewReader("data")); err != nil {
		t.Fatal(err)
	}

	err := fs.WriteMeta(ctx, "f.txt", map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	entry, _ := fs.Stat(ctx, "f.txt")
	if entry.Meta["key"] != "val" {
		t.Errorf("meta key = %q", entry.Meta["key"])
	}
	if entry.Meta["version"] != "1" {
		t.Errorf("WriteMeta should not bump version, got %q", entry.Meta["version"])
	}

	err = fs.WriteMeta(ctx, "ghost.txt", map[string]string{"k": "v"})
	if err == nil {
		t.Error("WriteMeta on nonexistent file should fail")
	}
}

func TestSQLiteFSMetaInOpen(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.WriteFile(ctx, "m.txt", []byte("hello"), map[string]string{"kind": "rss"}); err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open(ctx, "m.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	entry, _ := f.Stat()
	if entry.Meta["kind"] != "rss" {
		t.Errorf("Open meta kind = %q", entry.Meta["kind"])
	}
	if entry.Meta["version"] != "1" {
		t.Errorf("Open meta version = %q", entry.Meta["version"])
	}
}

func TestSQLiteFSPurge(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "old.txt", strings.NewReader("old")); err != nil {
		t.Fatal(err)
	}

	// Backdate the file to 2 hours ago
	_, err := fs.db.Exec(`UPDATE files SET modified = ? WHERE path = 'old.txt'`, time.Now().Add(-2*time.Hour).Unix())
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	if err := fs.Write(ctx, "new.txt", strings.NewReader("new")); err != nil {
		t.Fatal(err)
	}

	n, err := fs.Purge(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 1 {
		t.Errorf("Purge deleted %d, want 1", n)
	}

	_, err = fs.Stat(ctx, "old.txt")
	if err == nil {
		t.Error("old.txt should be purged")
	}
	_, err = fs.Stat(ctx, "new.txt")
	if err != nil {
		t.Error("new.txt should survive purge")
	}
}

func TestSQLiteFSPurgeByPrefix(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "feed/a.txt", strings.NewReader("a")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Write(ctx, "feed/b.txt", strings.NewReader("b")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Write(ctx, "other/c.txt", strings.NewReader("c")); err != nil {
		t.Fatal(err)
	}

	n, err := fs.PurgeByPrefix(ctx, "feed")
	if err != nil {
		t.Fatalf("PurgeByPrefix: %v", err)
	}
	if n != 2 {
		t.Errorf("PurgeByPrefix deleted %d, want 2", n)
	}

	_, err = fs.Stat(ctx, "feed/a.txt")
	if err == nil {
		t.Error("feed/a.txt should be purged")
	}
	_, err = fs.Stat(ctx, "other/c.txt")
	if err != nil {
		t.Error("other/c.txt should survive")
	}
}

func TestSQLiteFSTotalSizeAndCount(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "a.txt", strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	} // 5 bytes
	if err := fs.Write(ctx, "b.txt", strings.NewReader("world!!")); err != nil {
		t.Fatal(err)
	} // 7 bytes
	if err := fs.Mkdir(ctx, "dir", types.PermRWX); err != nil {
		t.Fatal(err)
	}

	size, err := fs.TotalSize(ctx)
	if err != nil {
		t.Fatalf("TotalSize: %v", err)
	}
	if size != 12 {
		t.Errorf("TotalSize = %d, want 12", size)
	}

	count, err := fs.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("Count = %d, want 2", count)
	}
}

func TestSQLiteFSMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	// Create a DB with the old schema (no version column)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT UNIQUE NOT NULL,
			content BLOB,
			is_dir BOOLEAN NOT NULL DEFAULT 0,
			perm INTEGER NOT NULL DEFAULT 1,
			modified INTEGER NOT NULL DEFAULT 0,
			meta TEXT
		)`)
	if err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	_, err = db.Exec(`INSERT INTO files (path, content, perm, modified) VALUES ('legacy.txt', 'old data', 1, 0)`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_ = db.Close()

	// Reopen via SQLiteFS — migration should add version column
	fs, err := NewSQLiteFS(dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("NewSQLiteFS after migration: %v", err)
	}
	defer func() { _ = fs.Close() }()

	entry, err := fs.Stat(context.Background(), "legacy.txt")
	if err != nil {
		t.Fatalf("Stat legacy file: %v", err)
	}
	if entry.Meta["version"] != "1" {
		t.Errorf("migrated version = %q, want 1", entry.Meta["version"])
	}
}
