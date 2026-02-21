# Tutorial: Cached Feeds (dbfs over httpfs)

This tutorial walks you through building a **cached RSS/Atom feed**: httpfs as the origin and dbfs (SQLite) as a read-through cache. By the end you’ll have a single mount that fills the cache on first read and serves from cache within TTL.

## What you’ll do

- Create an httpfs origin with one RSS/Atom source.
- Create a dbfs cache (SQLite).
- Compose them with `NewCachedUnion` and mount at `/feeds`.
- Run shell commands that hit the cache or the origin as appropriate.

## Prerequisites

- Go 1.24+
- The grasp module plus [dbfs](https://github.com/jackfish212/dbfs) and [httpfs](https://github.com/jackfish212/httpfs) (or use the example in the Shellfish repo, which uses `replace` in `go.mod`).

## Step 1: Open the cache and create the origin

Create a cache filesystem with dbfs and an httpfs origin with one feed:

```go
package main

import (
    "context"
    "time"

    "github.com/jackfish212/dbfs"
    "github.com/jackfish212/grasp"
    "github.com/jackfish212/grasp/builtins"
    "github.com/jackfish212/grasp/mounts"
    httpfs "github.com/jackfish212/httpfs"
    "github.com/jackfish212/grasp/types"

    _ "modernc.org/sqlite"
)

func main() {
    ctx := context.Background()

    cache, err := dbfs.Open("sqlite", "cache.db", types.PermRW)
    if err != nil {
        panic(err)
    }
    defer cache.Close()

    origin := httpfs.NewHTTPFS(httpfs.WithHTTPFSInterval(5 * time.Minute))
    if err := origin.Add("commits", "https://github.com/jackfish212/grasp/commits/main.atom", &httpfs.RSSParser{}); err != nil {
        panic(err)
    }
    origin.Start(ctx)
    defer origin.Stop()
```

- **cache** — Writable; later the union will backfill it when reading from origin.
- **origin** — Read-only from the shell’s point of view; httpfs fetches the feed and exposes it as files.

## Step 2: Build the union and mount it

Use `NewCachedUnion(cache, origin, ttl)` so that the cache is the first layer and the origin is the fallback. Then configure VirtualOS and mount the union at `/feeds`:

```go
    const ttl = 10 * time.Minute
    union := mounts.NewCachedUnion(cache, origin, ttl)

    v := grasp.New()
    rootFS, err := grasp.Configure(v)
    if err != nil {
        panic(err)
    }
    builtins.RegisterBuiltinsOnFS(v, rootFS)

    v.Mount("/feeds", union)
    shell := v.Shell("user")
```

## Step 3: Use the shell

First list the feed directory. The union will see no cache entries, so it will use the origin (httpfs). Reading a file triggers backfill into dbfs:

```go
    result := shell.Execute(ctx, "ls /feeds/commits")
    fmt.Println(result.Output)

    result = shell.Execute(ctx, "cat /feeds/commits/some-entry.txt")
    fmt.Println(result.Output)
```

- First `ls` and first `cat`: data comes from httpfs and is written into the cache.
- Subsequent reads within TTL: data comes from the cache.

## Step 4 (optional): Event-driven invalidation and background purge

To invalidate cache when httpfs sees changes, and to periodically purge old cache entries:

```go
    origin := httpfs.NewHTTPFS(
        httpfs.WithHTTPFSInterval(5*time.Minute),
        httpfs.WithHTTPFSOnEvent(func(ev types.EventType, path string) {
            _ = cache.Remove(ctx, path)
        }),
    )
    // ... Add source, then:

    union := mounts.NewCachedUnion(cache, origin, 10*time.Minute)
    union.StartPurge(15*time.Minute, func(ctx context.Context) error {
        _, err := cache.Purge(ctx, 10*time.Minute)
        return err
    })
    defer union.StopPurge()
```

- **OnEvent** — When httpfs updates a file, remove that path from the cache so the next read refetches and backfills.
- **StartPurge** — Every 15 minutes, call `cache.Purge(ctx, 10*time.Minute)` to delete cache files older than the TTL.

## Summary

- **Cache layer**: dbfs (SQLite), writable.
- **Origin layer**: httpfs with RSS/Atom, read-only.
- **Union**: `NewCachedUnion(cache, origin, ttl)`; mount once at e.g. `/feeds`.
- **Semantics**: First read → origin → backfill cache. Later reads within TTL → cache. Optional: event invalidation and background purge.

For a full runnable example, see [examples/cached-feeds/main.go](../../examples/cached-feeds/main.go). For concepts, see [Explanation: Union mount and cache](../explanation/union-mount-and-cache.md). For the API, see [Reference: Union mount](../reference/union-mount.md).
