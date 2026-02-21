package mounts

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackfish212/grasp/types"
)

func setupLocalFS(t *testing.T) (*LocalFS, string) {
	t.Helper()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("nested"), 0644)

	return NewLocalFS(dir, types.PermRW), dir
}

func TestLocalFSStat(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	entry, err := fs.Stat(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.Name != "hello.txt" {
		t.Errorf("Name = %q", entry.Name)
	}
	if entry.IsDir {
		t.Error("hello.txt should not be dir")
	}
	if entry.Size != 11 {
		t.Errorf("Size = %d, want 11", entry.Size)
	}
}

func TestLocalFSStatRoot(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	entry, err := fs.Stat(ctx, "")
	if err != nil {
		t.Fatalf("Stat root: %v", err)
	}
	if !entry.IsDir {
		t.Error("root should be dir")
	}
}

func TestLocalFSStatNotFound(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	_, err := fs.Stat(ctx, "nonexistent.txt")
	if err == nil {
		t.Error("Stat nonexistent should fail")
	}
}

func TestLocalFSList(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	entries, err := fs.List(ctx, "", types.ListOpts{})
	if err != nil {
		t.Fatalf("List root: %v", err)
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["hello.txt"] {
		t.Error("missing hello.txt")
	}
	if !names["sub"] {
		t.Error("missing sub/")
	}
}

func TestLocalFSListSubdir(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	entries, err := fs.List(ctx, "sub", types.ListOpts{})
	if err != nil {
		t.Fatalf("List sub: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "nested.txt" {
		t.Errorf("List sub = %v, want [nested.txt]", entries)
	}
}

func TestLocalFSOpen(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	f, err := fs.Open(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	data, _ := io.ReadAll(f)
	if string(data) != "hello world" {
		t.Errorf("content = %q", string(data))
	}
}

func TestLocalFSOpenNotFound(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	_, err := fs.Open(ctx, "ghost.txt")
	if err == nil {
		t.Error("Open nonexistent should fail")
	}
}

func TestLocalFSOpenReadOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)

	fs := NewLocalFS(dir, types.PermNone)
	ctx := context.Background()

	_, err := fs.Open(ctx, "file.txt")
	if err == nil {
		t.Error("Open on non-readable fs should fail")
	}
}

func TestLocalFSWrite(t *testing.T) {
	fs, dir := setupLocalFS(t)
	ctx := context.Background()

	err := fs.Write(ctx, "newfile.txt", strings.NewReader("new content"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "newfile.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "new content" {
		t.Errorf("written content = %q", string(data))
	}
}

func TestLocalFSWriteCreatesParent(t *testing.T) {
	fs, dir := setupLocalFS(t)
	ctx := context.Background()

	err := fs.Write(ctx, "deep/nested/file.txt", strings.NewReader("deep"))
	if err != nil {
		t.Fatalf("Write nested: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "deep", "nested", "file.txt"))
	if string(data) != "deep" {
		t.Errorf("content = %q", string(data))
	}
}

func TestLocalFSWriteReadOnly(t *testing.T) {
	dir := t.TempDir()
	fs := NewLocalFS(dir, types.PermRO)
	ctx := context.Background()

	err := fs.Write(ctx, "file.txt", strings.NewReader("data"))
	if err == nil {
		t.Error("Write on RO fs should fail")
	}
}

func TestLocalFSMkdir(t *testing.T) {
	fs, dir := setupLocalFS(t)
	ctx := context.Background()

	err := fs.Mkdir(ctx, "newdir", types.PermRWX)
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "newdir"))
	if err != nil {
		t.Fatalf("os.Stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("should be directory")
	}
}

func TestLocalFSRemove(t *testing.T) {
	fs, dir := setupLocalFS(t)
	ctx := context.Background()

	err := fs.Remove(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err = os.Stat(filepath.Join(dir, "hello.txt"))
	if !os.IsNotExist(err) {
		t.Error("file should be removed")
	}
}

func TestLocalFSRemoveNotFound(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	err := fs.Remove(ctx, "ghost.txt")
	if err == nil {
		t.Error("Remove nonexistent should fail")
	}
}

func TestLocalFSRename(t *testing.T) {
	fs, dir := setupLocalFS(t)
	ctx := context.Background()

	err := fs.Rename(ctx, "hello.txt", "renamed.txt")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "hello.txt")); !os.IsNotExist(err) {
		t.Error("old file should not exist")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "renamed.txt"))
	if string(data) != "hello world" {
		t.Errorf("renamed content = %q", string(data))
	}
}

func TestLocalFSSearch(t *testing.T) {
	fs, _ := setupLocalFS(t)
	ctx := context.Background()

	results, err := fs.Search(ctx, "hello", types.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search returned %d results, want 1", len(results))
	}
	if results[0].Entry.Name != "hello.txt" {
		t.Errorf("Search result name = %q", results[0].Entry.Name)
	}
}

func TestLocalFSSearchMaxResults(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		name := "match_" + string(rune('a'+i)) + ".txt"
		os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644)
	}

	fs := NewLocalFS(dir, types.PermRW)
	ctx := context.Background()

	results, err := fs.Search(ctx, "match", types.SearchOpts{MaxResults: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("Search should respect MaxResults, got %d", len(results))
	}
}

func TestLocalFSMountInfo(t *testing.T) {
	dir := t.TempDir()
	fs := NewLocalFS(dir, types.PermRW)
	name, extra := fs.MountInfo()
	if name != "localfs" {
		t.Errorf("MountInfo name = %q", name)
	}
	if extra == "" {
		t.Error("MountInfo extra should not be empty")
	}
}
