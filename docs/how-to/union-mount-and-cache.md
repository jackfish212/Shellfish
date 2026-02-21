# How to use union mount and cache

This guide shows how to combine multiple providers at one path (union mount) and how to use a cache layer with TTL, event-driven invalidation, and background purge.

## Using the `bind` command (shell)

If your shell has the `bind` builtin (via `builtins.RegisterBuiltinsOnFS` or `RegisterBuiltins`), you can build a union from existing mount points.

**Syntax:**

```text
bind [-b|-a] source_path target_path
```

- **`bind -b /cache /data`** — Put the provider at `/cache` **before** the provider at `/data`. The result is a union mounted at `/data`: lookups check `/cache` first, then `/data`. Use this to add a cache layer on top of `/data`.
- **`bind -a /fallback /data`** — Put the provider at `/fallback` **after** `/data`. Lookups check `/data` first, then `/fallback`. Use this to add a fallback or origin under `/data`.
- **`bind /new /data`** — Replace the provider at `/data` with the provider at `/new` (no union).

**Requirements:**

- Both `source_path` and `target_path` must be **exact mount points** (no path under a mount). For example, `/feeds` is OK; `/feeds/commits` is not.

**Example (conceptual):**

```text
mount /feeds httpfs
mount /cache dbfs
bind -b /cache /feeds
ls /feeds/commits
```

Here `/feeds` would show a union of cache (first) and httpfs (second). In practice you usually build the union in code with `NewCachedUnion` and mount it once; `bind` is useful when you already have separate mounts and want to combine them without code changes.

## Building a cached union in code

**1. Create cache and origin providers.**

- Cache: must implement `Provider` and `Writable` (e.g. dbfs). Optionally `Mutable` for `Remove`/`Purge`.
- Origin: must implement `Provider` and `Readable` (e.g. httpfs).

**2. Compose with `NewCachedUnion`.**

```go
union := mounts.NewCachedUnion(cache, origin, 10*time.Minute)
v.Mount("/feeds", union)
```

Order is fixed: cache layer first, origin second. TTL applies only to the cache layer (entries older than `Modified + TTL` are treated as missing and the union falls through to the origin).

**3. Optional: event-driven invalidation.**

When the origin can notify on change (e.g. httpfs `WithHTTPFSOnEvent`), call the cache’s `Remove` for the changed path so the next read refetches and backfills:

```go
origin := httpfs.NewHTTPFS(
    httpfs.WithHTTPFSOnEvent(func(ev types.EventType, path string) {
        if m, ok := cache.(types.Mutable); ok {
            _ = m.Remove(ctx, path)
        }
    }),
)
```

If `cache` is concrete (e.g. `*dbfs.FS`), you can call `cache.Remove(ctx, path)` directly.

**4. Optional: background purge.**

To periodically delete old cache entries (e.g. by age), use `StartPurge` with a function that calls the cache’s purge:

```go
union.StartPurge(15*time.Minute, func(ctx context.Context) error {
    _, err := cache.Purge(ctx, 10*time.Minute)
    return err
})
defer union.StopPurge()
```

This runs every 15 minutes and purges cache files older than 10 minutes. Adjust intervals and TTL to your needs.

## Adding layers dynamically with `Bind`

If you already have a `*mounts.UnionProvider` (e.g. from `NewUnion` or `NewCachedUnion`), you can add more layers at runtime:

```go
u := mounts.NewUnion(
    mounts.Layer{Provider: cache, Mode: mounts.BindBefore, Cache: true, TTL: 10*time.Minute},
    mounts.Layer{Provider: origin, Mode: mounts.BindAfter},
)
u.Bind(anotherProvider, mounts.BindAfter, mounts.WithCache(5*time.Minute))
```

- `BindBefore` — Insert the new provider at the front (checked first).
- `BindAfter` — Append (checked last among current layers).
- `BindReplace` — Replace all layers with this one.
- `WithCache(ttl)` — Mark the new layer as a cache layer with the given TTL.

## Summary

| Goal | Approach |
|------|----------|
| Cache in front of origin at one path | `NewCachedUnion(cache, origin, ttl)` and mount once. |
| Combine two existing mounts from the shell | `bind -b /cache /data` or `bind -a /fallback /data`. |
| Invalidate on origin change | Use origin’s event callback and call `cache.Remove(ctx, path)`. |
| Limit cache size/age | `union.StartPurge(interval, func(){ cache.Purge(ctx, olderThan) })`. |
| Add another layer to an existing union | `u.Bind(provider, BindBefore|BindAfter, opts...)`. |

See [Explanation: Union mount and cache](../explanation/union-mount-and-cache.md) for concepts and [Reference: Union mount](../reference/union-mount.md) for the full API.
