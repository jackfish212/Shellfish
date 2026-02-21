# Union Mount and Cache Layer

This document explains the concepts behind GRASP’s Plan 9–style union mounting and how caching is implemented as a layer configuration rather than a new filesystem abstraction.

## Why union mount?

In Plan 9, the `bind` command attaches one directory tree onto another at the same path. Lookups then see a **union**: the system checks the bound (source) tree first, then falls back to the target. That gives:

- **Layering** — Multiple data sources appear under one path.
- **Override / fallback** — “Source before target” = override; “source after target” = fallback.
- **Composition** — The same namespace can combine local files, remote APIs, and caches without changing the core mount table or VirtualOS.

GRASP adopts this idea: a **UnionProvider** is a single `Provider` that composes several providers (layers) at one logical path. The mount table still sees one provider; the union decides which layer answers each request.

## Cache as configuration, not a new interface

Caching behavior is **not** a new capability (no new interface). It is a **per-layer configuration**:

- A layer can be marked as a **cache layer** (`Cache: true`) with an optional **TTL**.
- On read, the union tries cache layers first; if an entry exists and is **fresh** (`Modified + TTL > now`), it returns that. Otherwise it skips to the next layer.
- When data is read from a non-cache layer (the “origin”), the union can **backfill** the first writable cache layer by calling the existing `Writable.Write()`.

So:

- **Freshness** uses the existing `Entry.Modified` field.
- **Backfill** uses the existing `Writable` interface.
- **No new types** are required on the Provider/Readable/Writable contracts.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  VirtualOS / MountTable                                      │
│  e.g. Mount("/feeds", union)                                 │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  UnionProvider (single Provider)                             │
│  Stat/List/Open/Write → dispatch over layers                  │
└───────────────────────────┬─────────────────────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
┌───────────────┐  ┌───────────────┐  ┌───────────────┐
│  Layer 0      │  │  Layer 1      │  │  ...          │
│  (BEFORE)     │  │  (AFTER)      │  │               │
│  e.g. dbfs    │  │  e.g. httpfs  │  │               │
│  Cache: true  │  │  Cache: false │  │               │
│  TTL: 10m     │  │  (origin)     │  │               │
└───────────────┘  └───────────────┘  └───────────────┘
```

**Read path (read-through cache):**

1. For **Stat** or **Open**, the union walks layers in order.
2. On a **cache layer**, it only “hits” if the entry exists and (when TTL &gt; 0) is not expired.
3. If no cache layer returns a fresh result, the union tries **non-cache** (origin) layers.
4. On an origin hit for **Open**, the union reads the content, **backfills** the first writable cache layer, then returns the data.

So: first read fills the cache; later reads within TTL are served from the cache.

## Three invalidation strategies

Cache invalidation uses existing semantics only:

| Strategy | When | How |
|----------|------|-----|
| **Passive TTL** | On every read | Union checks `Entry.Modified + TTL > now`; if expired, skips cache and goes to origin, then backfills. |
| **Active (event-driven)** | When origin reports change | e.g. httpfs `WithHTTPFSOnEvent` callback calls `cache.Remove(ctx, path)` so the next read refetches and backfills. |
| **Background purge** | Periodically | Union can run a ticker that calls e.g. `dbfs.Purge(ctx, olderThan)` to delete old cache files. |

All three can be used together: TTL at read time, event-driven invalidation when the origin changes, and background purge to cap cache size or age.

## Design principles (summary)

- **No new interfaces** — Reuse `Provider`, `Readable`, `Writable`, `Mutable`, and `Entry.Modified`.
- **No core changes** — MountTable and VirtualOS stay unchanged; union is “just” another provider.
- **Plan 9 spirit** — `bind` command, union semantics, and namespace composition.
- **Cache = configuration** — Cache behavior is “this layer is a cache with this TTL,” not a new FS kind.
- **Three-way invalidation** — Passive TTL, active events, and background Purge.

See also:

- [Reference: Union Mount](../reference/union-mount.md) — API for `UnionProvider`, `Layer`, `BindMode`, and the `bind` command.
- [How-to: Union mount and cache](../how-to/union-mount-and-cache.md) — Practical setup and bind usage.
- [Tutorial: Cached feeds](../tutorials/cached-feeds.md) — Step-by-step cached httpfs RSS example.
