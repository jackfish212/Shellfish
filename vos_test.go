package afs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/agentfs/afs/mounts"
)

func setupVOS(t *testing.T) *VirtualOS {
	t.Helper()
	v := New()
	root := mounts.NewMemFS(PermRW)
	v.Mount("/", root)
	root.AddDir("bin")
	root.AddDir("home")
	root.AddDir("home/agent")
	root.AddFile("home/agent/notes.txt", []byte("my notes"), PermRW)
	return v
}

func TestVOSStat(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	entry, err := v.Stat(ctx, "/home/agent/notes.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.Name != "notes.txt" {
		t.Errorf("Name = %q", entry.Name)
	}
	if entry.Path != "/home/agent/notes.txt" {
		t.Errorf("Path = %q, want /home/agent/notes.txt", entry.Path)
	}
}

func TestVOSStatVirtualDir(t *testing.T) {
	v := New()
	v.Mount("/data", mounts.NewMemFS(PermRW))
	v.Mount("/tools", mounts.NewMemFS(PermRW))

	ctx := context.Background()
	entry, err := v.Stat(ctx, "/")
	if err != nil {
		t.Fatalf("Stat /: %v", err)
	}
	if !entry.IsDir {
		t.Error("/ should be dir")
	}
}

func TestVOSStatNotFound(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	_, err := v.Stat(ctx, "/nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestVOSListMergesChildMounts(t *testing.T) {
	v := New()
	root := mounts.NewMemFS(PermRW)
	v.Mount("/", root)
	root.AddFile("readme.md", []byte("hi"), PermRO)
	root.AddDir("data")

	data := mounts.NewMemFS(PermRW)
	v.Mount("/data", data)

	ctx := context.Background()
	entries, err := v.List(ctx, "/", ListOpts{})
	if err != nil {
		t.Fatalf("List /: %v", err)
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["readme.md"] {
		t.Error("missing readme.md from root provider")
	}
	if !names["data"] {
		t.Error("missing data from child mount")
	}
}

func TestVOSListVirtualDirOnly(t *testing.T) {
	v := New()
	root := mounts.NewMemFS(PermRW)
	v.Mount("/", root)
	root.AddDir("data")
	v.Mount("/data/sub1", mounts.NewMemFS(PermRW))
	v.Mount("/data/sub2", mounts.NewMemFS(PermRW))

	ctx := context.Background()
	entries, err := v.List(ctx, "/data", ListOpts{})
	if err != nil {
		t.Fatalf("List /data: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestVOSListNotFound(t *testing.T) {
	v := New()
	ctx := context.Background()

	_, err := v.List(ctx, "/nowhere", ListOpts{})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestVOSOpenAndRead(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	f, err := v.Open(ctx, "/home/agent/notes.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "my notes" {
		t.Errorf("content = %q", string(data))
	}
}

func TestVOSOpenNotReadable(t *testing.T) {
	v := New()
	stub := &stubProvider{}
	v.Mount("/ro", stub)

	ctx := context.Background()
	_, err := v.Open(ctx, "/ro/file")
	if !errors.Is(err, ErrNotReadable) {
		t.Errorf("expected ErrNotReadable, got: %v", err)
	}
}

func TestVOSWrite(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	err := v.Write(ctx, "/home/agent/new.txt", strings.NewReader("new content"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	f, err := v.Open(ctx, "/home/agent/new.txt")
	if err != nil {
		t.Fatalf("Open after Write: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "new content" {
		t.Errorf("content = %q", string(data))
	}
}

func TestVOSWriteNotWritable(t *testing.T) {
	v := New()
	v.Mount("/ro", &stubProvider{})

	ctx := context.Background()
	err := v.Write(ctx, "/ro/file", strings.NewReader("data"))
	if !errors.Is(err, ErrNotWritable) {
		t.Errorf("expected ErrNotWritable, got: %v", err)
	}
}

func TestVOSExec(t *testing.T) {
	v := New()
	fs := mounts.NewMemFS(PermRW)
	v.Mount("/", fs)
	fs.AddDir("bin")

	fs.AddExecFunc("bin/greet", func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		name := "world"
		if len(args) > 0 {
			name = args[0]
		}
		return io.NopCloser(strings.NewReader("hello " + name + "\n")), nil
	}, mounts.FuncMeta{Description: "greet"})

	ctx := context.Background()
	rc, err := v.Exec(ctx, "/bin/greet", []string{"Alice"}, nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "hello Alice\n" {
		t.Errorf("output = %q", string(data))
	}
}

func TestVOSExecNotExecutable(t *testing.T) {
	v := New()
	v.Mount("/ro", &stubProvider{})

	ctx := context.Background()
	_, err := v.Exec(ctx, "/ro/cmd", nil, nil)
	if !errors.Is(err, ErrNotExecutable) {
		t.Errorf("expected ErrNotExecutable, got: %v", err)
	}
}

func TestVOSMkdir(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	err := v.Mkdir(ctx, "/home/agent/subdir", PermRWX)
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	entry, err := v.Stat(ctx, "/home/agent/subdir")
	if err != nil {
		t.Fatalf("Stat after Mkdir: %v", err)
	}
	if !entry.IsDir {
		t.Error("should be directory")
	}
}

func TestVOSRemove(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	err := v.Remove(ctx, "/home/agent/notes.txt")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err = v.Stat(ctx, "/home/agent/notes.txt")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after remove, got: %v", err)
	}
}

func TestVOSRenameSameMount(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	err := v.Rename(ctx, "/home/agent/notes.txt", "/home/agent/renamed.txt")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	_, err = v.Stat(ctx, "/home/agent/notes.txt")
	if err == nil {
		t.Error("old path should not exist")
	}
	_, err = v.Stat(ctx, "/home/agent/renamed.txt")
	if err != nil {
		t.Errorf("new path should exist: %v", err)
	}
}

func TestVOSRenameCrossMount(t *testing.T) {
	v := New()
	v.Mount("/a", mounts.NewMemFS(PermRW))
	v.Mount("/b", mounts.NewMemFS(PermRW))

	ctx := context.Background()
	v.Write(ctx, "/a/file.txt", strings.NewReader("data"))

	err := v.Rename(ctx, "/a/file.txt", "/b/file.txt")
	if !errors.Is(err, ErrNotSupported) {
		t.Errorf("cross-mount rename should fail with ErrNotSupported, got: %v", err)
	}
}

func TestVOSSearch(t *testing.T) {
	v := New()
	local := mounts.NewMemFS(PermRW)
	v.Mount("/data", local)
	local.AddFile("report.txt", []byte("quarterly report"), PermRO)

	searchable := mounts.NewLocalFS(t.TempDir(), PermRW)
	v.Mount("/fs", searchable)

	ctx := context.Background()
	results, err := v.Search(ctx, "report", SearchOpts{Scope: "/data"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	_ = results
}

func TestVOSSearchNoSearchableProviders(t *testing.T) {
	v := New()
	v.Mount("/data", mounts.NewMemFS(PermRW))

	ctx := context.Background()
	results, err := v.Search(ctx, "anything", SearchOpts{})
	if err != nil {
		t.Fatalf("Search should not error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestVOSMountDuplicate(t *testing.T) {
	v := New()
	v.Mount("/data", mounts.NewMemFS(PermRW))

	err := v.Mount("/data", mounts.NewMemFS(PermRW))
	if !errors.Is(err, ErrAlreadyMounted) {
		t.Errorf("duplicate mount should fail: %v", err)
	}
}

func TestVOSUnmount(t *testing.T) {
	v := New()
	v.Mount("/data", mounts.NewMemFS(PermRW))

	err := v.Unmount("/data")
	if err != nil {
		t.Fatalf("Unmount: %v", err)
	}

	ctx := context.Background()
	_, err = v.Stat(ctx, "/data")
	if err == nil {
		t.Error("Stat should fail after unmount")
	}
}

func TestVOSOpenFile(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	f, err := v.OpenFile(ctx, "/home/agent/notes.txt", O_RDONLY)
	if err != nil {
		t.Fatalf("OpenFile O_RDONLY: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if string(data) != "my notes" {
		t.Errorf("content = %q", string(data))
	}
}

func TestVOSOpenFileWrite(t *testing.T) {
	v := setupVOS(t)
	ctx := context.Background()

	f, err := v.OpenFile(ctx, "/home/agent/writable.txt", O_WRONLY|O_CREATE)
	if err != nil {
		t.Fatalf("OpenFile O_WRONLY: %v", err)
	}
	w, ok := f.(io.Writer)
	if !ok {
		t.Fatal("writable file should implement io.Writer")
	}
	w.Write([]byte("written via OpenFile"))
	f.Close()

	f2, err := v.Open(ctx, "/home/agent/writable.txt")
	if err != nil {
		t.Fatalf("Open after write: %v", err)
	}
	defer f2.Close()
	data, _ := io.ReadAll(f2)
	if string(data) != "written via OpenFile" {
		t.Errorf("content = %q", string(data))
	}
}

func TestVOSShell(t *testing.T) {
	v := setupVOS(t)
	sh := v.Shell("agent")
	if sh == nil {
		t.Fatal("Shell() returned nil")
	}
	if sh.Env.Get("USER") != "agent" {
		t.Errorf("USER = %q, want agent", sh.Env.Get("USER"))
	}
}

// TestVOSMountWithoutPreExistingDir tests that mounting to a path that doesn't
// exist in the parent filesystem should succeed. Mount points are virtual directories.
func TestVOSMountWithoutPreExistingDir(t *testing.T) {
	v := New()
	root := mounts.NewMemFS(PermRW)
	if err := v.Mount("/", root); err != nil {
		t.Fatalf("Mount /: %v", err)
	}

	// Mount /data WITHOUT creating "data" directory in root memfs first
	data := mounts.NewMemFS(PermRW)
	if err := v.Mount("/data", data); err != nil {
		t.Fatalf("Mount /data should succeed without pre-existing dir: %v", err)
	}

	// Verify /data exists as a virtual directory
	ctx := context.Background()
	entry, err := v.Stat(ctx, "/data")
	if err != nil {
		t.Fatalf("Stat /data: %v", err)
	}
	if !entry.IsDir {
		t.Error("/data should be a directory")
	}

	// Verify we can write to /data
	if err := v.Write(ctx, "/data/test.txt", strings.NewReader("hello")); err != nil {
		t.Fatalf("Write /data/test.txt: %v", err)
	}

	// Verify we can read from /data
	f, err := v.Open(ctx, "/data/test.txt")
	if err != nil {
		t.Fatalf("Open /data/test.txt: %v", err)
	}
	defer f.Close()
	data2, _ := io.ReadAll(f)
	if string(data2) != "hello" {
		t.Errorf("content = %q, want hello", string(data2))
	}
}
