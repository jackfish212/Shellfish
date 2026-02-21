package mounts

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackfish212/grasp/types"
)

func TestUnionStatFirstLayerWins(t *testing.T) {
	ctx := context.Background()
	top := NewMemFS(types.PermRW)
	bot := NewMemFS(types.PermRW)
	top.AddFile("a.txt", []byte("from top"), types.PermRO)
	bot.AddFile("a.txt", []byte("from bot"), types.PermRO)

	u := NewUnion(
		Layer{Provider: top, Mode: BindBefore},
		Layer{Provider: bot, Mode: BindAfter},
	)

	entry, err := u.Stat(ctx, "a.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.Name != "a.txt" {
		t.Errorf("Name = %q", entry.Name)
	}

	f, err := u.Open(ctx, "a.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(f)
	if string(data) != "from top" {
		t.Errorf("content = %q, want from top", string(data))
	}
}

func TestUnionListMergesLayers(t *testing.T) {
	ctx := context.Background()
	top := NewMemFS(types.PermRW)
	bot := NewMemFS(types.PermRW)
	top.AddFile("a.txt", []byte("a"), types.PermRO)
	bot.AddFile("b.txt", []byte("b"), types.PermRO)

	u := NewUnion(
		Layer{Provider: top, Mode: BindBefore},
		Layer{Provider: bot, Mode: BindAfter},
	)

	entries, err := u.List(ctx, "", types.ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List len = %d, want 2", len(entries))
	}
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("expected a.txt and b.txt, got %v", names)
	}
}

func TestUnionListFirstOccurrenceWins(t *testing.T) {
	ctx := context.Background()
	top := NewMemFS(types.PermRW)
	bot := NewMemFS(types.PermRW)
	top.AddFile("same.txt", []byte("first"), types.PermRO)
	bot.AddFile("same.txt", []byte("second"), types.PermRO)

	u := NewUnion(
		Layer{Provider: top, Mode: BindBefore},
		Layer{Provider: bot, Mode: BindAfter},
	)

	entries, err := u.List(ctx, "", types.ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List len = %d, want 1", len(entries))
	}
	if entries[0].Name != "same.txt" {
		t.Errorf("Name = %q", entries[0].Name)
	}
	f, _ := u.Open(ctx, "same.txt")
	data, _ := io.ReadAll(f)
	_ = f.Close()
	if string(data) != "first" {
		t.Errorf("content = %q, want first", string(data))
	}
}

func TestCachedUnionReadThroughAndBackfill(t *testing.T) {
	ctx := context.Background()
	cache := NewMemFS(types.PermRW)
	origin := NewMemFS(types.PermRW)
	origin.AddFile("cached.txt", []byte("from origin"), types.PermRO)

	u := NewCachedUnion(cache, origin, 10*time.Minute)

	f, err := u.Open(ctx, "cached.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	data, _ := io.ReadAll(f)
	_ = f.Close()
	if string(data) != "from origin" {
		t.Errorf("first Open content = %q", string(data))
	}

	_, err = cache.Stat(ctx, "cached.txt")
	if err != nil {
		t.Errorf("cache should have cached.txt after backfill: %v", err)
	}
	f2, _ := cache.Open(ctx, "cached.txt")
	data2, _ := io.ReadAll(f2)
	_ = f2.Close()
	if string(data2) != "from origin" {
		t.Errorf("cache content = %q", string(data2))
	}

	f3, err := u.Open(ctx, "cached.txt")
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	data3, _ := io.ReadAll(f3)
	_ = f3.Close()
	if string(data3) != "from origin" {
		t.Errorf("second Open content = %q", string(data3))
	}
}

func TestCachedUnionTTLExpiry(t *testing.T) {
	ctx := context.Background()
	cache := NewMemFS(types.PermRW)
	origin := NewMemFS(types.PermRW)
	origin.AddFile("ttl.txt", []byte("fresh"), types.PermRO)

	u := NewCachedUnion(cache, origin, 5*time.Millisecond)

	f, _ := u.Open(ctx, "ttl.txt")
	_, _ = io.ReadAll(f)
	_ = f.Close()

	time.Sleep(10 * time.Millisecond)

	f2, err := u.Open(ctx, "ttl.txt")
	if err != nil {
		t.Fatalf("Open after TTL: %v", err)
	}
	data, _ := io.ReadAll(f2)
	_ = f2.Close()
	if string(data) != "fresh" {
		t.Errorf("content after TTL = %q", string(data))
	}
}

func TestUnionWriteGoesToFirstWritable(t *testing.T) {
	ctx := context.Background()
	top := NewMemFS(types.PermRW)
	bot := NewMemFS(types.PermRW)

	u := NewUnion(
		Layer{Provider: top, Mode: BindBefore},
		Layer{Provider: bot, Mode: BindAfter},
	)

	err := u.Write(ctx, "w.txt", strings.NewReader("written"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	_, err = top.Stat(ctx, "w.txt")
	if err != nil {
		t.Errorf("top should have w.txt: %v", err)
	}
	_, err = bot.Stat(ctx, "w.txt")
	if err == nil {
		t.Error("bot should not have w.txt")
	}
}

func TestUnionMkdirRemove(t *testing.T) {
	ctx := context.Background()
	top := NewMemFS(types.PermRW)
	bot := NewMemFS(types.PermRW)
	top.AddFile("f.txt", []byte("x"), types.PermRW)

	u := NewUnion(
		Layer{Provider: top, Mode: BindBefore},
		Layer{Provider: bot, Mode: BindAfter},
	)

	if err := u.Mkdir(ctx, "dir", types.PermRW); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	entry, err := top.Stat(ctx, "dir")
	if err != nil || !entry.IsDir {
		t.Errorf("top should have dir: %v", err)
	}

	if err := u.Remove(ctx, "f.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	_, err = top.Stat(ctx, "f.txt")
	if err == nil {
		t.Error("f.txt should be removed from top")
	}
}

func TestUnionBind(t *testing.T) {
	ctx := context.Background()
	u := NewUnion(Layer{Provider: NewMemFS(types.PermRO), Mode: BindBefore})
	mem := NewMemFS(types.PermRW)
	mem.AddFile("b.txt", []byte("b"), types.PermRO)

	u.Bind(mem, BindAfter)

	entry, err := u.Stat(ctx, "b.txt")
	if err != nil {
		t.Fatalf("Stat after Bind: %v", err)
	}
	if entry.Name != "b.txt" {
		t.Errorf("Name = %q", entry.Name)
	}
}

func TestUnionMountInfo(t *testing.T) {
	u := NewUnion(
		Layer{Provider: NewMemFS(types.PermRO), Mode: BindBefore},
		Layer{Provider: NewMemFS(types.PermRO), Mode: BindAfter},
	)
	name, extra := u.MountInfo()
	if name != "union" {
		t.Errorf("name = %q", name)
	}
	if extra != "2 layers" {
		t.Errorf("extra = %q", extra)
	}
}

func TestUnionStartPurge(t *testing.T) {
	var called atomic.Int32
	u := NewUnion(Layer{Provider: NewMemFS(types.PermRO), Mode: BindBefore})

	u.StartPurge(10*time.Millisecond, func(context.Context) error {
		called.Add(1)
		return nil
	})
	defer u.StopPurge()

	time.Sleep(35 * time.Millisecond)
	n := called.Load()
	if n < 2 {
		t.Errorf("purge callback called %d times, want at least 2", n)
	}
}

func TestUnionStatNotFound(t *testing.T) {
	ctx := context.Background()
	u := NewUnion(Layer{Provider: NewMemFS(types.PermRO), Mode: BindBefore})

	_, err := u.Stat(ctx, "nonexistent")
	if err != types.ErrNotFound {
		t.Errorf("Stat = %v, want ErrNotFound", err)
	}
}

func TestUnionOpenNotFound(t *testing.T) {
	ctx := context.Background()
	u := NewUnion(Layer{Provider: NewMemFS(types.PermRO), Mode: BindBefore})

	_, err := u.Open(ctx, "nonexistent")
	if err != types.ErrNotFound {
		t.Errorf("Open = %v, want ErrNotFound", err)
	}
}

func TestUnionWriteNoWritable(t *testing.T) {
	ctx := context.Background()
	u := NewUnion(Layer{Provider: NewMemFS(types.PermRO), Mode: BindBefore})

	err := u.Write(ctx, "x", strings.NewReader("data"))
	if !errors.Is(err, types.ErrNotWritable) {
		t.Errorf("Write = %v, want ErrNotWritable", err)
	}
}
