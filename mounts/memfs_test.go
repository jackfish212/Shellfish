package mounts

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jackfish212/shellfish/types"
)

func TestMemFSAddFileAndOpen(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddFile("hello.txt", []byte("hello world"), types.PermRO)

	ctx := context.Background()

	entry, err := fs.Stat(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.Name != "hello.txt" {
		t.Errorf("Name = %q, want %q", entry.Name, "hello.txt")
	}
	if entry.IsDir {
		t.Error("should not be a directory")
	}

	f, err := fs.Open(ctx, "hello.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "hello world" {
		t.Errorf("content = %q, want %q", string(data), "hello world")
	}
}

func TestMemFSAddDir(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddDir("docs")

	ctx := context.Background()
	entry, err := fs.Stat(ctx, "docs")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !entry.IsDir {
		t.Error("should be a directory")
	}
}

func TestMemFSList(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddFile("a.txt", []byte("a"), types.PermRO)
	fs.AddFile("b.txt", []byte("b"), types.PermRO)
	fs.AddFile("sub/c.txt", []byte("c"), types.PermRO)

	ctx := context.Background()
	entries, err := fs.List(ctx, "", types.ListOpts{})
	if err != nil {
		t.Fatalf("List root: %v", err)
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("List root missing files: %v", entries)
	}
	if !names["sub"] {
		t.Error("List root should show 'sub' as implicit directory")
	}
}

func TestMemFSListSubdirectory(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddFile("sub/file1.txt", []byte("1"), types.PermRO)
	fs.AddFile("sub/file2.txt", []byte("2"), types.PermRO)

	ctx := context.Background()
	entries, err := fs.List(ctx, "sub", types.ListOpts{})
	if err != nil {
		t.Fatalf("List sub: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("List sub returned %d entries, want 2", len(entries))
	}
}

func TestMemFSListNotFound(t *testing.T) {
	fs := NewMemFS(types.PermRW)

	ctx := context.Background()
	_, err := fs.List(ctx, "nonexistent", types.ListOpts{})
	if err == nil {
		t.Error("List nonexistent should return error")
	}
}

func TestMemFSWrite(t *testing.T) {
	fs := NewMemFS(types.PermRW)

	ctx := context.Background()
	err := fs.Write(ctx, "new.txt", strings.NewReader("new content"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := fs.Open(ctx, "new.txt")
	if err != nil {
		t.Fatalf("Open after Write: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "new content" {
		t.Errorf("content = %q, want %q", string(data), "new content")
	}
}

func TestMemFSWriteOverwrite(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()

	fs.Write(ctx, "file.txt", strings.NewReader("v1"))
	fs.Write(ctx, "file.txt", strings.NewReader("v2"))

	f, _ := fs.Open(ctx, "file.txt")
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "v2" {
		t.Errorf("overwritten content = %q, want %q", string(data), "v2")
	}
}

func TestMemFSWriteReadOnly(t *testing.T) {
	fs := NewMemFS(types.PermRO)
	ctx := context.Background()

	err := fs.Write(ctx, "test.txt", strings.NewReader("data"))
	if err == nil {
		t.Error("Write on RO fs should fail")
	}
}

func TestMemFSExecFunc(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddExecFunc("greet", func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		name := "world"
		if len(args) > 0 {
			name = args[0]
		}
		return io.NopCloser(strings.NewReader("hello " + name)), nil
	}, FuncMeta{Description: "Greet someone"})

	ctx := context.Background()

	entry, err := fs.Stat(ctx, "greet")
	if err != nil {
		t.Fatalf("Stat greet: %v", err)
	}
	if !entry.Perm.CanExec() {
		t.Error("greet should be executable")
	}

	rc, err := fs.Exec(ctx, "greet", []string{"Alice"}, nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "hello Alice" {
		t.Errorf("Exec output = %q, want %q", string(data), "hello Alice")
	}
}

func TestMemFSAddFunc(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddFunc("old", func(ctx context.Context, args []string, stdin string) (string, error) {
		return "legacy:" + stdin, nil
	}, FuncMeta{Description: "Legacy func"})

	ctx := context.Background()
	rc, err := fs.Exec(ctx, "old", nil, strings.NewReader("input"))
	if err != nil {
		t.Fatalf("Exec old func: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "legacy:input" {
		t.Errorf("output = %q, want %q", string(data), "legacy:input")
	}
}

func TestMemFSOpenExecutableReturnHelp(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddExecFunc("tool", func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("result")), nil
	}, FuncMeta{Description: "A tool", Usage: "tool [args]"})

	ctx := context.Background()
	f, err := fs.Open(ctx, "tool")
	if err != nil {
		t.Fatalf("Open tool: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "A tool") {
		t.Errorf("opening executable should return help, got: %q", string(data))
	}
}

func TestMemFSRemoveFunc(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddExecFunc("cmd", func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("")), nil
	}, FuncMeta{Description: "temp"})

	if !fs.RemoveFunc("cmd") {
		t.Error("RemoveFunc should return true")
	}
	if fs.RemoveFunc("cmd") {
		t.Error("RemoveFunc of removed should return false")
	}

	ctx := context.Background()
	_, err := fs.Stat(ctx, "cmd")
	if err == nil {
		t.Error("Stat after RemoveFunc should fail")
	}
}

func TestMemFSMkdir(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()

	if err := fs.Mkdir(ctx, "newdir", types.PermRWX); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	entry, err := fs.Stat(ctx, "newdir")
	if err != nil {
		t.Fatalf("Stat after Mkdir: %v", err)
	}
	if !entry.IsDir {
		t.Error("should be directory")
	}
}

func TestMemFSMkdirDuplicate(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()
	fs.Mkdir(ctx, "dir", types.PermRWX)

	err := fs.Mkdir(ctx, "dir", types.PermRWX)
	if err == nil {
		t.Error("duplicate Mkdir should fail")
	}
}

func TestMemFSRemove(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()

	fs.AddFile("removeme.txt", []byte("bye"), types.PermRW)
	if err := fs.Remove(ctx, "removeme.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err := fs.Stat(ctx, "removeme.txt")
	if err == nil {
		t.Error("Stat after Remove should fail")
	}
}

func TestMemFSRemoveRecursive(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()

	fs.AddDir("parent")
	fs.AddFile("parent/child.txt", []byte("c"), types.PermRW)
	fs.AddFile("parent/sub/deep.txt", []byte("d"), types.PermRW)

	if err := fs.Remove(ctx, "parent"); err != nil {
		t.Fatalf("Remove parent: %v", err)
	}

	_, err := fs.Stat(ctx, "parent/child.txt")
	if err == nil {
		t.Error("children should be removed too")
	}
}

func TestMemFSRemoveNotFound(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()
	err := fs.Remove(ctx, "ghost")
	if err == nil {
		t.Error("Remove nonexistent should fail")
	}
}

func TestMemFSRename(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()

	fs.AddFile("old.txt", []byte("content"), types.PermRW)
	if err := fs.Rename(ctx, "old.txt", "new.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	_, err := fs.Stat(ctx, "old.txt")
	if err == nil {
		t.Error("old path should not exist after rename")
	}
	entry, err := fs.Stat(ctx, "new.txt")
	if err != nil {
		t.Fatalf("Stat new.txt: %v", err)
	}
	if entry.Name != "new.txt" {
		t.Errorf("Name = %q, want %q", entry.Name, "new.txt")
	}
}

func TestMemFSRenameWithChildren(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()

	fs.AddDir("dir1")
	fs.AddFile("dir1/a.txt", []byte("a"), types.PermRW)

	if err := fs.Rename(ctx, "dir1", "dir2"); err != nil {
		t.Fatalf("Rename dir: %v", err)
	}

	_, err := fs.Stat(ctx, "dir2/a.txt")
	if err != nil {
		t.Error("child should be renamed too")
	}
}

func TestMemFSStatImplicitDir(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddFile("docs/readme.md", []byte("hi"), types.PermRO)

	ctx := context.Background()
	entry, err := fs.Stat(ctx, "docs")
	if err != nil {
		t.Fatalf("Stat implicit dir: %v", err)
	}
	if !entry.IsDir {
		t.Error("implicit dir should be IsDir=true")
	}
}

func TestMemFSStatNotFound(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	ctx := context.Background()
	_, err := fs.Stat(ctx, "nothing")
	if err == nil {
		t.Error("Stat nonexistent should fail")
	}
}

func TestMemFSMountInfo(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	name, extra := fs.MountInfo()
	if name != "memfs" {
		t.Errorf("MountInfo name = %q, want %q", name, "memfs")
	}
	if extra != "in-memory" {
		t.Errorf("MountInfo extra = %q, want %q", extra, "in-memory")
	}
}

func TestMemFSWriteToFunc(t *testing.T) {
	fs := NewMemFS(types.PermRW)
	fs.AddExecFunc("cmd", func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("")), nil
	}, FuncMeta{Description: "test"})

	ctx := context.Background()
	err := fs.Write(ctx, "cmd", strings.NewReader("overwrite"))
	if err == nil {
		t.Error("writing to a func entry should fail")
	}
}
