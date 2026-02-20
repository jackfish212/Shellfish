package shellfish

import (
	"context"
	"errors"
	"testing"

	"github.com/jackfish212/shellfish/types"
)

type stubProvider struct{}

func (s *stubProvider) Stat(_ context.Context, path string) (*types.Entry, error) {
	return &types.Entry{Name: path, Path: path, IsDir: true, Perm: PermRX}, nil
}
func (s *stubProvider) List(_ context.Context, _ string, _ types.ListOpts) ([]types.Entry, error) {
	return nil, nil
}

func TestMountTableMountAndResolve(t *testing.T) {
	mt := NewMountTable()
	p := &stubProvider{}

	if err := mt.Mount("/data", p); err != nil {
		t.Fatalf("Mount /data: %v", err)
	}

	got, inner, err := mt.Resolve("/data")
	if err != nil {
		t.Fatalf("Resolve /data: %v", err)
	}
	if got != p {
		t.Error("Resolve returned wrong provider")
	}
	if inner != "" {
		t.Errorf("inner = %q, want empty", inner)
	}

	got, inner, err = mt.Resolve("/data/file.txt")
	if err != nil {
		t.Fatalf("Resolve /data/file.txt: %v", err)
	}
	if got != p {
		t.Error("Resolve returned wrong provider for nested path")
	}
	if inner != "file.txt" {
		t.Errorf("inner = %q, want %q", inner, "file.txt")
	}

	got, inner, err = mt.Resolve("/data/sub/deep")
	if err != nil {
		t.Fatalf("Resolve /data/sub/deep: %v", err)
	}
	if inner != "sub/deep" {
		t.Errorf("inner = %q, want %q", inner, "sub/deep")
	}
}

func TestMountTableRootMount(t *testing.T) {
	mt := NewMountTable()
	p := &stubProvider{}

	if err := mt.Mount("/", p); err != nil {
		t.Fatalf("Mount /: %v", err)
	}

	got, inner, err := mt.Resolve("/anything")
	if err != nil {
		t.Fatalf("Resolve /anything: %v", err)
	}
	if got != p {
		t.Error("wrong provider")
	}
	if inner != "anything" {
		t.Errorf("inner = %q, want %q", inner, "anything")
	}
}

func TestMountTableLongestPrefix(t *testing.T) {
	mt := NewMountTable()
	pRoot := &stubProvider{}
	pData := &stubProvider{}

	mt.Mount("/", pRoot)
	mt.Mount("/data", pData)

	got, _, _ := mt.Resolve("/data/file")
	if got != pData {
		t.Error("should resolve to /data provider, not root")
	}

	got, _, _ = mt.Resolve("/other")
	if got != pRoot {
		t.Error("should resolve to root provider")
	}
}

func TestMountTableDuplicate(t *testing.T) {
	mt := NewMountTable()
	p := &stubProvider{}
	mt.Mount("/data", p)

	err := mt.Mount("/data", p)
	if !errors.Is(err, ErrAlreadyMounted) {
		t.Errorf("duplicate mount should return ErrAlreadyMounted, got: %v", err)
	}
}

func TestMountTableUnmount(t *testing.T) {
	mt := NewMountTable()
	p := &stubProvider{}
	mt.Mount("/data", p)

	if err := mt.Unmount("/data"); err != nil {
		t.Fatalf("Unmount /data: %v", err)
	}

	_, _, err := mt.Resolve("/data")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("after unmount, Resolve should fail, got: %v", err)
	}
}

func TestMountTableUnmountNotFound(t *testing.T) {
	mt := NewMountTable()
	err := mt.Unmount("/nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("unmount non-existent should return ErrNotFound, got: %v", err)
	}
}

func TestMountTableChildMounts(t *testing.T) {
	mt := NewMountTable()
	mt.Mount("/data", &stubProvider{})
	mt.Mount("/tools", &stubProvider{})
	mt.Mount("/data/sub", &stubProvider{})

	children := mt.ChildMounts("/")
	names := make(map[string]bool)
	for _, c := range children {
		names[c.Name] = true
	}
	if !names["data"] || !names["tools"] {
		t.Errorf("ChildMounts(/) = %v, want data and tools", children)
	}

	children = mt.ChildMounts("/data")
	if len(children) != 1 || children[0].Name != "sub" {
		t.Errorf("ChildMounts(/data) = %v, want [sub]", children)
	}
}

func TestMountTableAll(t *testing.T) {
	mt := NewMountTable()
	mt.Mount("/a", &stubProvider{})
	mt.Mount("/b", &stubProvider{})

	paths := mt.All()
	if len(paths) != 2 {
		t.Errorf("All() returned %d paths, want 2", len(paths))
	}
}

func TestMountTableAllInfo(t *testing.T) {
	mt := NewMountTable()
	mt.Mount("/data", &stubProvider{})

	infos := mt.AllInfo()
	if len(infos) != 1 {
		t.Fatalf("AllInfo() returned %d, want 1", len(infos))
	}
	if infos[0].Path != "/data" {
		t.Errorf("AllInfo[0].Path = %q, want /data", infos[0].Path)
	}
	if infos[0].Permissions != "---" {
		t.Errorf("stubProvider should show --- permissions, got %q", infos[0].Permissions)
	}
}

func TestMountTableResolveCache(t *testing.T) {
	mt := NewMountTable()
	p := &stubProvider{}
	mt.Mount("/data", p)

	mt.Resolve("/data/file")
	got, inner, err := mt.Resolve("/data/file")
	if err != nil {
		t.Fatalf("cached resolve failed: %v", err)
	}
	if got != p || inner != "file" {
		t.Error("cached resolve returned wrong result")
	}

	mt.Unmount("/data")
	_, _, err = mt.Resolve("/data/file")
	if err == nil {
		t.Error("cache should be invalidated after unmount")
	}
}
