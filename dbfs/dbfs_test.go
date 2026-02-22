package dbfs

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

func setup(t *testing.T) *FS {
	t.Helper()
	dir := t.TempDir()
	fs, err := Open("sqlite", filepath.Join(dir, "test.db"), types.PermRW)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { fs.Close() })
	return fs
}

func mustWrite(t *testing.T, fs *FS, ctx context.Context, path, content string) {
	t.Helper()
	if err := fs.Write(ctx, path, strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}
}

func TestWriteAndOpen(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	if err := fs.Write(ctx, "hello.txt", strings.NewReader("hello dbfs")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := fs.Open(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "hello dbfs" {
		t.Errorf("content = %q, want %q", string(data), "hello dbfs")
	}
}

func TestStat(t *testing.T) {
	fs := setup(t)
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

func TestStatImplicitDir(t *testing.T) {
	fs := setup(t)
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

func TestStatNotFound(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	_, err := fs.Stat(ctx, "ghost.txt")
	if err == nil {
		t.Error("Stat nonexistent should fail")
	}
}

func TestStatRoot(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	entry, err := fs.Stat(ctx, "")
	if err != nil {
		t.Fatalf("Stat root: %v", err)
	}
	if !entry.IsDir {
		t.Error("root should be dir")
	}
}

func TestList(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "a.txt", "a")
	mustWrite(t, fs, ctx, "b.txt", "b")
	mustWrite(t, fs, ctx, "sub/c.txt", "c")

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

func TestListSubdir(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "dir/file1.txt", "1")
	mustWrite(t, fs, ctx, "dir/file2.txt", "2")

	entries, err := fs.List(ctx, "dir", types.ListOpts{})
	if err != nil {
		t.Fatalf("List dir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("List dir returned %d entries, want 2", len(entries))
	}
}

func TestListNotFound(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	_, err := fs.List(ctx, "nonexistent", types.ListOpts{})
	if err == nil {
		t.Error("List nonexistent should fail")
	}
}

func TestOverwrite(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "file.txt", "v1")
	mustWrite(t, fs, ctx, "file.txt", "v2")

	f, _ := fs.Open(ctx, "file.txt")
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "v2" {
		t.Errorf("overwrite = %q, want v2", string(data))
	}
}

func TestWriteReadOnly(t *testing.T) {
	dir := t.TempDir()
	fs, err := Open("sqlite", filepath.Join(dir, "ro.db"), types.PermRO)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fs.Close()

	err = fs.Write(context.Background(), "file.txt", strings.NewReader("data"))
	if err == nil {
		t.Error("Write on RO fs should fail")
	}
}

func TestMkdir(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	if err := fs.Mkdir(ctx, "newdir", types.PermRWX); err != nil {
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

func TestRemove(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "removeme.txt", "bye")
	if err := fs.Remove(ctx, "removeme.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := fs.Stat(ctx, "removeme.txt"); err == nil {
		t.Error("Stat after Remove should fail")
	}
}

func TestRemoveRecursive(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	if err := fs.Mkdir(ctx, "parent", types.PermRWX); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, fs, ctx, "parent/child.txt", "c")

	if err := fs.Remove(ctx, "parent"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := fs.Stat(ctx, "parent/child.txt"); err == nil {
		t.Error("children should be removed")
	}
}

func TestRemoveNotFound(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	if err := fs.Remove(ctx, "ghost.txt"); err == nil {
		t.Error("Remove nonexistent should fail")
	}
}

func TestRename(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "old.txt", "content")
	if err := fs.Rename(ctx, "old.txt", "new.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if _, err := fs.Stat(ctx, "old.txt"); err == nil {
		t.Error("old should not exist")
	}
	f, err := fs.Open(ctx, "new.txt")
	if err != nil {
		t.Fatalf("Open new.txt: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "content" {
		t.Errorf("renamed content = %q", string(data))
	}
}

func TestRenameNotFound(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	if err := fs.Rename(ctx, "ghost.txt", "new.txt"); err == nil {
		t.Error("Rename nonexistent should fail")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "persist.db")

	fs1, err := Open("sqlite", dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx := context.Background()
	mustWrite(t, fs1, ctx, "persistent.txt", "survive restart")
	fs1.Close()

	fs2, err := Open("sqlite", dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("Open reopen: %v", err)
	}
	defer fs2.Close()

	f, err := fs2.Open(ctx, "persistent.txt")
	if err != nil {
		t.Fatalf("Open after reopen: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "survive restart" {
		t.Errorf("persisted content = %q", string(data))
	}
}

func TestMountInfo(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "info.db")
	fs, _ := Open("sqlite", dbPath, types.PermRW)
	defer fs.Close()

	name, extra := fs.MountInfo()
	if name != "dbfs" {
		t.Errorf("MountInfo name = %q", name)
	}
	if extra != dbPath {
		t.Errorf("MountInfo extra = %q, want %q", extra, dbPath)
	}
}

func TestInvalidPath(t *testing.T) {
	_, err := Open("sqlite", filepath.Join(os.DevNull, "impossible", "path.db"), types.PermRW)
	if err == nil {
		t.Error("Open with invalid path should fail")
	}
}

func TestVersion(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "f.txt", "v1")
	e1, _ := fs.Stat(ctx, "f.txt")
	if e1.Meta["version"] != "1" {
		t.Errorf("version after first write = %q, want 1", e1.Meta["version"])
	}

	mustWrite(t, fs, ctx, "f.txt", "v2")
	e2, _ := fs.Stat(ctx, "f.txt")
	if e2.Meta["version"] != "2" {
		t.Errorf("version after second write = %q, want 2", e2.Meta["version"])
	}

	mustWrite(t, fs, ctx, "f.txt", "v3")
	e3, _ := fs.Stat(ctx, "f.txt")
	if e3.Meta["version"] != "3" {
		t.Errorf("version after third write = %q, want 3", e3.Meta["version"])
	}
}

func TestWriteFileWithMeta(t *testing.T) {
	fs := setup(t)
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

func TestWriteMeta(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "f.txt", "data")

	if err := fs.WriteMeta(ctx, "f.txt", map[string]string{"key": "val"}); err != nil {
		t.Fatalf("WriteMeta: %v", err)
	}

	entry, _ := fs.Stat(ctx, "f.txt")
	if entry.Meta["key"] != "val" {
		t.Errorf("meta key = %q", entry.Meta["key"])
	}
	if entry.Meta["version"] != "1" {
		t.Errorf("WriteMeta should not bump version, got %q", entry.Meta["version"])
	}

	if err := fs.WriteMeta(ctx, "ghost.txt", map[string]string{"k": "v"}); err == nil {
		t.Error("WriteMeta on nonexistent file should fail")
	}
}

func TestMetaInOpen(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	if err := fs.WriteFile(ctx, "m.txt", []byte("hello"), map[string]string{"kind": "rss"}); err != nil {
		t.Fatal(err)
	}

	f, err := fs.Open(ctx, "m.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	entry, _ := f.Stat()
	if entry.Meta["kind"] != "rss" {
		t.Errorf("Open meta kind = %q", entry.Meta["kind"])
	}
	if entry.Meta["version"] != "1" {
		t.Errorf("Open meta version = %q", entry.Meta["version"])
	}
}

func TestPurge(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "old.txt", "old")
	_, _ = fs.db.Exec(fs.q(`UPDATE {t} SET modified = ? WHERE path = 'old.txt'`), time.Now().Add(-2*time.Hour).Unix())
	mustWrite(t, fs, ctx, "new.txt", "new")

	n, err := fs.Purge(ctx, 1*time.Hour)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 1 {
		t.Errorf("Purge deleted %d, want 1", n)
	}
	if _, err := fs.Stat(ctx, "old.txt"); err == nil {
		t.Error("old.txt should be purged")
	}
	if _, err := fs.Stat(ctx, "new.txt"); err != nil {
		t.Error("new.txt should survive purge")
	}
}

func TestPurgeByPrefix(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "feed/a.txt", "a")
	mustWrite(t, fs, ctx, "feed/b.txt", "b")
	mustWrite(t, fs, ctx, "other/c.txt", "c")

	n, err := fs.PurgeByPrefix(ctx, "feed")
	if err != nil {
		t.Fatalf("PurgeByPrefix: %v", err)
	}
	if n != 2 {
		t.Errorf("PurgeByPrefix deleted %d, want 2", n)
	}
	if _, err := fs.Stat(ctx, "feed/a.txt"); err == nil {
		t.Error("feed/a.txt should be purged")
	}
	if _, err := fs.Stat(ctx, "other/c.txt"); err != nil {
		t.Error("other/c.txt should survive")
	}
}

func TestTotalSizeAndCount(t *testing.T) {
	fs := setup(t)
	ctx := context.Background()

	mustWrite(t, fs, ctx, "a.txt", "hello")   // 5 bytes
	mustWrite(t, fs, ctx, "b.txt", "world!!") // 7 bytes
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

func TestMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "migrate.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE files (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			path     TEXT UNIQUE NOT NULL,
			content  BLOB,
			is_dir   INTEGER NOT NULL DEFAULT 0,
			perm     INTEGER NOT NULL DEFAULT 1,
			modified INTEGER NOT NULL DEFAULT 0,
			meta     TEXT
		)`)
	if err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	_, err = db.Exec(`INSERT INTO files (path, content, perm, modified) VALUES ('legacy.txt', 'old data', 1, 0)`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	db.Close()

	fs, err := Open("sqlite", dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("Open after migration: %v", err)
	}
	defer fs.Close()

	entry, err := fs.Stat(context.Background(), "legacy.txt")
	if err != nil {
		t.Fatalf("Stat legacy file: %v", err)
	}
	if entry.Meta["version"] != "1" {
		t.Errorf("migrated version = %q, want 1", entry.Meta["version"])
	}
}

func TestCustomTableName(t *testing.T) {
	dir := t.TempDir()
	fs, err := Open("sqlite", filepath.Join(dir, "custom.db"), types.PermRW, Table("cache"))
	if err != nil {
		t.Fatalf("Open with custom table: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	mustWrite(t, fs, ctx, "test.txt", "custom table")

	f, err := fs.Open(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "custom table" {
		t.Errorf("content = %q", string(data))
	}
}

func TestInvalidTableName(t *testing.T) {
	dir := t.TempDir()
	_, err := Open("sqlite", filepath.Join(dir, "bad.db"), types.PermRW, Table("drop table;"))
	if err == nil {
		t.Error("should reject invalid table name")
	}
}

func TestUnknownDriver(t *testing.T) {
	_, err := Open("mysql", "localhost", types.PermRW)
	if err == nil {
		t.Error("should reject unknown driver")
	}
}

func TestOpenDB(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "opendb.db"))
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	fs, err := OpenDB(db, "sqlite", types.PermRW)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer fs.Close()

	ctx := context.Background()
	mustWrite(t, fs, ctx, "test.txt", "via OpenDB")
	f, _ := fs.Open(ctx, "test.txt")
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "via OpenDB" {
		t.Errorf("content = %q", string(data))
	}
}

func TestRebind(t *testing.T) {
	pg := PostgresDialect{}
	got := pg.Rebind(`SELECT * FROM t WHERE a = ? AND b = ? AND c LIKE ?`)
	want := `SELECT * FROM t WHERE a = $1 AND b = $2 AND c LIKE $3`
	if got != want {
		t.Errorf("Rebind:\n got  %q\n want %q", got, want)
	}

	sq := SQLiteDialect{}
	orig := `SELECT * FROM t WHERE a = ?`
	if sq.Rebind(orig) != orig {
		t.Error("SQLite Rebind should be identity")
	}
}
