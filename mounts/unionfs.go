// Package mounts provides built-in Mount implementations for grasp.
//
// unionfs.go implements Plan 9-style union mounting: multiple providers
// are layered at the same logical path. Cache behavior (TTL, read-through
// backfill) is configured per layer without new filesystem semantics.
package mounts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/jackfish212/grasp/types"
)

var (
	_ types.Provider         = (*UnionProvider)(nil)
	_ types.Readable         = (*UnionProvider)(nil)
	_ types.Writable        = (*UnionProvider)(nil)
	_ types.Mutable         = (*UnionProvider)(nil)
	_ types.Touchable       = (*UnionProvider)(nil)
	_ types.MountInfoProvider = (*UnionProvider)(nil)
)

// BindMode is the Plan 9 bind direction for a union layer.
type BindMode int

const (
	BindBefore  BindMode = iota // new layer in front (cache/override)
	BindAfter                   // new layer behind (fallback/origin)
	BindReplace                 // replace existing (used when building union)
)

// Layer is one filesystem in a union stack.
type Layer struct {
	Provider types.Provider
	Mode     BindMode

	// Cache config: when true, Stat/Open treat this as a cache layer.
	// TTL: if > 0, entries older than Modified+TTL are considered expired and skipped.
	Cache bool
	TTL   time.Duration
}

// LayerOption configures a layer (e.g. when calling Bind).
type LayerOption func(*Layer)

// WithCache marks the layer as a cache layer with the given TTL (0 = never expire).
func WithCache(ttl time.Duration) LayerOption {
	return func(l *Layer) {
		l.Cache = true
		l.TTL = ttl
	}
}

// UnionProvider composes multiple providers at the same path with Plan 9 union semantics.
type UnionProvider struct {
	mu      sync.RWMutex
	layers  []Layer
	purge   *time.Ticker
	done    chan struct{}
	purgeFn func(context.Context) error
}

// NewUnion creates a union from the given layers. Order is preserved (first layer is checked first).
func NewUnion(layers ...Layer) *UnionProvider {
	u := &UnionProvider{layers: make([]Layer, 0, len(layers))}
	for _, l := range layers {
		u.layers = append(u.layers, l)
	}
	return u
}

// NewCachedUnion creates a two-layer union: cache on top, origin below, with read-through and TTL.
func NewCachedUnion(cache types.Provider, origin types.Provider, ttl time.Duration) *UnionProvider {
	return NewUnion(
		Layer{Provider: cache, Mode: BindBefore, Cache: true, TTL: ttl},
		Layer{Provider: origin, Mode: BindAfter},
	)
}

// Bind adds a provider to the union with the given mode and options.
func (u *UnionProvider) Bind(p types.Provider, mode BindMode, opts ...LayerOption) {
	u.mu.Lock()
	defer u.mu.Unlock()

	l := Layer{Provider: p, Mode: mode}
	for _, opt := range opts {
		opt(&l)
	}

	switch mode {
	case BindBefore:
		u.layers = append([]Layer{l}, u.layers...)
	case BindAfter:
		u.layers = append(u.layers, l)
	case BindReplace:
		u.layers = []Layer{l}
	}
}

// Stat returns the first matching entry across layers. Cache layers skip expired entries.
func (u *UnionProvider) Stat(ctx context.Context, path string) (*types.Entry, error) {
	path = normPath(path)
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	for _, layer := range layers {
		entry, err := layer.Provider.Stat(ctx, path)
		if err != nil {
			continue
		}
		if layer.Cache && layer.TTL > 0 {
			if !entry.Modified.IsZero() && time.Since(entry.Modified) > layer.TTL {
				continue
			}
		}
		if entry.Path == "" || !strings.HasPrefix(entry.Path, "/") {
			entry.Path = path
		}
		return entry, nil
	}
	return nil, types.ErrNotFound
}

// List merges entries from all layers; first occurrence of each name wins.
func (u *UnionProvider) List(ctx context.Context, path string, opts types.ListOpts) ([]types.Entry, error) {
	path = normPath(path)
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	var merged []types.Entry
	seen := make(map[string]bool)
	prefix := path
	if prefix != "" {
		prefix += "/"
	}

	for _, layer := range layers {
		entries, err := layer.Provider.List(ctx, path, opts)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if seen[e.Name] {
				continue
			}
			seen[e.Name] = true
			if e.Path == "" || !strings.HasPrefix(e.Path, "/") {
				e.Path = prefix + e.Name
			}
			merged = append(merged, e)
		}
	}

	if len(merged) == 0 && path != "" {
		return nil, types.ErrNotFound
	}
	return merged, nil
}

// Open implements read-through cache: try cache layers first (fresh only), then origin; on miss from origin, backfill to first writable cache layer.
func (u *UnionProvider) Open(ctx context.Context, path string) (types.File, error) {
	path = normPath(path)
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	// 1) Try cache layers only (fresh entries); return if hit
	for _, layer := range layers {
		if !layer.Cache {
			continue
		}
		r, ok := layer.Provider.(types.Readable)
		if !ok {
			continue
		}
		entry, err := layer.Provider.Stat(ctx, path)
		if err != nil {
			continue
		}
		if entry.IsDir {
			continue
		}
		if layer.TTL > 0 {
			if !entry.Modified.IsZero() && time.Since(entry.Modified) > layer.TTL {
				continue
			}
		}
		f, err := r.Open(ctx, path)
		if err != nil {
			continue
		}
		if entry.Path == "" {
			entry.Path = path
		}
		return types.NewFile(path, entry, f), nil
	}

	// 1b) Try non-cache readable layers (return first hit without backfill if read-only)
	for _, layer := range layers {
		if layer.Cache {
			continue
		}
		r, ok := layer.Provider.(types.Readable)
		if !ok {
			continue
		}
		entry, err := layer.Provider.Stat(ctx, path)
		if err != nil {
			continue
		}
		if entry.IsDir {
			continue
		}
		f, err := r.Open(ctx, path)
		if err != nil {
			continue
		}
		// Origin hit: backfill to first writable cache layer, then return
		data, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			continue
		}
		u.backfill(ctx, path, data)
		if entry.Path == "" {
			entry.Path = path
		}
		return types.NewFile(path, entry, io.NopCloser(bytes.NewReader(data))), nil
	}

	return nil, types.ErrNotFound
}

// backfill writes data to the first writable cache layer.
func (u *UnionProvider) backfill(ctx context.Context, path string, data []byte) {
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	for _, layer := range layers {
		if !layer.Cache {
			continue
		}
		w, ok := layer.Provider.(types.Writable)
		if !ok {
			continue
		}
		_ = w.Write(ctx, path, bytes.NewReader(data))
		return
	}
}

// Write writes to the first writable layer.
func (u *UnionProvider) Write(ctx context.Context, path string, r io.Reader) error {
	path = normPath(path)
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	for _, layer := range layers {
		if w, ok := layer.Provider.(types.Writable); ok {
			return w.Write(ctx, path, r)
		}
	}
	return types.ErrNotWritable
}

// Mkdir creates the directory in the first mutable layer.
func (u *UnionProvider) Mkdir(ctx context.Context, path string, perm types.Perm) error {
	path = normPath(path)
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	for _, layer := range layers {
		if m, ok := layer.Provider.(types.Mutable); ok {
			return m.Mkdir(ctx, path, perm)
		}
	}
	return types.ErrNotSupported
}

// Remove removes from the first mutable layer that has the entry.
func (u *UnionProvider) Remove(ctx context.Context, path string) error {
	path = normPath(path)
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	for _, layer := range layers {
		m, ok := layer.Provider.(types.Mutable)
		if !ok {
			continue
		}
		if _, err := layer.Provider.Stat(ctx, path); err != nil {
			continue
		}
		return m.Remove(ctx, path)
	}
	return types.ErrNotFound
}

// Rename renames within the first mutable layer that has the source (cross-layer rename not supported).
func (u *UnionProvider) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPath = normPath(oldPath)
	newPath = normPath(newPath)
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	for _, layer := range layers {
		m, ok := layer.Provider.(types.Mutable)
		if !ok {
			continue
		}
		if _, err := layer.Provider.Stat(ctx, oldPath); err != nil {
			continue
		}
		return m.Rename(ctx, oldPath, newPath)
	}
	return types.ErrNotFound
}

// Touch updates mtime on the first Touchable layer that has the entry, else first Writable.
func (u *UnionProvider) Touch(ctx context.Context, path string) error {
	path = normPath(path)
	u.mu.RLock()
	layers := make([]Layer, len(u.layers))
	copy(layers, u.layers)
	u.mu.RUnlock()

	for _, layer := range layers {
		if t, ok := layer.Provider.(types.Touchable); ok {
			if _, err := layer.Provider.Stat(ctx, path); err != nil {
				continue
			}
			return t.Touch(ctx, path)
		}
	}
	for _, layer := range layers {
		if w, ok := layer.Provider.(types.Writable); ok {
			if r, ok := layer.Provider.(types.Readable); ok {
				f, err := r.Open(ctx, path)
				if err != nil {
					continue
				}
				data, _ := io.ReadAll(f)
				_ = f.Close()
				return w.Write(ctx, path, bytes.NewReader(data))
			}
			return w.Write(ctx, path, bytes.NewReader(nil))
		}
	}
	return types.ErrNotSupported
}

// MountInfo implements types.MountInfoProvider.
func (u *UnionProvider) MountInfo() (name, extra string) {
	u.mu.RLock()
	n := len(u.layers)
	u.mu.RUnlock()
	return "union", fmt.Sprintf("%d layers", n)
}

// StartPurge runs purgeFunc periodically. purgeFunc is typically dbfs.Purge(ctx, olderThan).
// Call StopPurge to stop.
func (u *UnionProvider) StartPurge(interval time.Duration, purgeFunc func(context.Context) error) {
	u.mu.Lock()
	if u.purge != nil {
		u.mu.Unlock()
		return
	}
	u.purgeFn = purgeFunc
	u.done = make(chan struct{})
	ticker := time.NewTicker(interval)
	u.purge = ticker
	done := u.done
	u.mu.Unlock()

	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				u.mu.RLock()
				f := u.purgeFn
				u.mu.RUnlock()
				if f != nil {
					_ = f(context.Background())
				}
			}
		}
	}()
}

// StopPurge stops the background purge goroutine.
func (u *UnionProvider) StopPurge() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.purge != nil {
		u.purge.Stop()
		u.purge = nil
	}
	if u.done != nil {
		close(u.done)
		u.done = nil
	}
	u.purgeFn = nil
}
