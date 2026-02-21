# Union Mount API Reference

Package: `github.com/jackfish212/grasp/mounts`

This page describes the types and functions for Plan 9–style union mounting and the builtin `bind` command.

---

## Types

### BindMode

Direction of a layer in the union (Plan 9 bind semantics).

```go
type BindMode int

const (
    BindBefore  BindMode = iota  // layer in front (checked first); e.g. cache/override
    BindAfter                    // layer behind (checked last); e.g. fallback/origin
    BindReplace                  // replace all layers with this one
)
```

### Layer

One filesystem in a union stack.

```go
type Layer struct {
    Provider types.Provider
    Mode     BindMode

    // Cache: when true, Stat/Open treat this as a cache layer.
    // TTL: if > 0, entries with Modified older than TTL are skipped (fall through to next layer).
    Cache bool
    TTL   time.Duration
}
```

### LayerOption

Optional configuration for a layer (e.g. when calling `Bind`).

```go
type LayerOption func(*Layer)
```

### UnionProvider

Composes multiple providers at one path. Implements `types.Provider`, `Readable`, `Writable`, `Mutable`, `Touchable`, and `MountInfoProvider`.

```go
type UnionProvider struct {
    // ... internal fields
}
```

**Behavior:**

- **Stat** — Returns the first layer that has the path; for cache layers with TTL &gt; 0, skips entries where `time.Since(entry.Modified) > TTL`.
- **List** — Merges entries from all layers; first occurrence of each name wins.
- **Open** — Tries cache layers first (fresh entries only); then non-cache (origin) layers. On an origin hit, backfills the first writable cache layer and returns the data.
- **Write** — Writes to the first layer that implements `Writable`.
- **Mkdir / Remove / Rename** — Act on the first layer that implements `Mutable` and has the entry (or for Mkdir, any mutable layer).
- **Touch** — Updates mtime on the first `Touchable` layer that has the entry, or writes through the first writable layer.
- **MountInfo** — Returns `"union"` and a string like `"2 layers"`.

---

## Constructors and functions

### NewUnion

Creates a union from the given layers. Order is preserved; the first layer is checked first.

```go
func NewUnion(layers ...Layer) *UnionProvider
```

### NewCachedUnion

Convenience constructor for a two-layer union: cache on top, origin below, with read-through and TTL.

```go
func NewCachedUnion(cache types.Provider, origin types.Provider, ttl time.Duration) *UnionProvider
```

Equivalent to:

```go
NewUnion(
    Layer{Provider: cache, Mode: BindBefore, Cache: true, TTL: ttl},
    Layer{Provider: origin, Mode: BindAfter},
)
```

### WithCache

Layer option that marks the layer as a cache with the given TTL. `ttl == 0` means never expire.

```go
func WithCache(ttl time.Duration) LayerOption
```

### Bind

Adds a provider to the union with the given mode and options. Safe for concurrent use.

```go
func (u *UnionProvider) Bind(p types.Provider, mode BindMode, opts ...LayerOption)
```

- **BindBefore** — Prepends the new layer.
- **BindAfter** — Appends the new layer.
- **BindReplace** — Replaces all layers with this one.

### StartPurge

Starts a background goroutine that calls `purgeFunc` at the given interval. Typical use: `purgeFunc` calls `cache.Purge(ctx, olderThan)`. Call `StopPurge` when done.

```go
func (u *UnionProvider) StartPurge(interval time.Duration, purgeFunc func(context.Context) error)
```

If `StartPurge` is already running, this call is a no-op.

### StopPurge

Stops the background purge goroutine and clears the purge function.

```go
func (u *UnionProvider) StopPurge()
```

---

## Shell command: `bind`

When builtins are registered (e.g. `builtins.RegisterBuiltinsOnFS` or `RegisterBuiltins`), the shell provides a `bind` command.

**Usage:**

```text
bind [-b|-a] source_path target_path
```

**Flags:**

| Flag | Meaning |
|------|--------|
| `-b` | Bind source **before** target: source layer is on top (e.g. cache). |
| `-a` | Bind source **after** target: source layer is below (e.g. fallback). |
| (none) | Replace target with source (single provider at target). |

**Arguments:**

- **source_path** — Path of the existing mount to use as the source (must be an exact mount point).
- **target_path** — Path of the existing mount to use as the target (must be an exact mount point). After `bind`, the target path is re-mounted with a `UnionProvider` that combines source and target (or, with no flag, target is replaced by source).

**Requirements:**

- Both paths must resolve to **exact mount points** (no path under a mount). Example: `/feeds` is OK; `/feeds/commits` is not.
- The implementation unmounts the target, builds a new union from target’s provider and source’s provider, then mounts the union at the target path.

**Help:**

```text
bind -h
bind --help
```

---

## See also

- [Explanation: Union mount and cache](../explanation/union-mount-and-cache.md)
- [How-to: Union mount and cache](../how-to/union-mount-and-cache.md)
- [Tutorial: Cached feeds](../tutorials/cached-feeds.md)
