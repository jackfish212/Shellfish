package mounts

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentfs/afs/types"
)

func setupSQLiteFS(t *testing.T) *SQLiteFS {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	fs, err := NewSQLiteFS(dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("NewSQLiteFS: %v", err)
	}
	t.Cleanup(func() { fs.Close() })
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
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "hello sqlite" {
		t.Errorf("content = %q, want %q", string(data), "hello sqlite")
	}
}

func TestSQLiteFSStat(t *testing.T) {
	fs := setupSQLiteFS(t)
	ctx := context.Background()

	fs.Write(ctx, "file.txt", strings.NewReader("data"))

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

	fs.Write(ctx, "docs/readme.md", strings.NewReader("hello"))

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

	fs.Write(ctx, "a.txt", strings.NewReader("a"))
	fs.Write(ctx, "b.txt", strings.NewReader("b"))
	fs.Write(ctx, "sub/c.txt", strings.NewReader("c"))

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

	fs.Write(ctx, "dir/file1.txt", strings.NewReader("1"))
	fs.Write(ctx, "dir/file2.txt", strings.NewReader("2"))

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

	fs.Write(ctx, "file.txt", strings.NewReader("v1"))
	fs.Write(ctx, "file.txt", strings.NewReader("v2"))

	f, _ := fs.Open(ctx, "file.txt")
	defer f.Close()
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
	defer fs.Close()

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

	fs.Write(ctx, "removeme.txt", strings.NewReader("bye"))
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

	fs.Mkdir(ctx, "parent", types.PermRWX)
	fs.Write(ctx, "parent/child.txt", strings.NewReader("c"))

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

	fs.Write(ctx, "old.txt", strings.NewReader("content"))
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
	defer f.Close()
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
	fs1.Write(ctx, "persistent.txt", strings.NewReader("survive restart"))
	fs1.Close()

	fs2, err := NewSQLiteFS(dbPath, types.PermRW)
	if err != nil {
		t.Fatalf("NewSQLiteFS reopen: %v", err)
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

func TestSQLiteFSMountInfo(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "info.db")
	fs, _ := NewSQLiteFS(dbPath, types.PermRW)
	defer fs.Close()

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
