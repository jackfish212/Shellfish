# Provider Model

The Provider model is Shellfish's core extensibility mechanism. It defines how external data sources, tools, and services plug into the virtual filesystem.

## Design Philosophy

Most plugin systems use one of two approaches:

1. **Fat interface.** Every plugin must implement every method (read, write, execute, search, ...), returning "not supported" for things it can't do. This is wasteful and error-prone.

2. **Registration-based.** Plugins register themselves for specific operations via a registry. This creates coupling to the registry and makes capability discovery opaque.

Shellfish uses a third approach: **interface composition with runtime detection.**

The base `Provider` interface is minimal — just `Stat` and `List`. Additional capabilities are separate interfaces that a provider *may* implement:

```go
type Provider interface {
    Stat(ctx context.Context, path string) (*Entry, error)
    List(ctx context.Context, path string, opts ListOpts) ([]Entry, error)
}

type Readable interface {
    Open(ctx context.Context, path string) (File, error)
}

type Writable interface {
    Write(ctx context.Context, path string, r io.Reader) error
}

type Executable interface {
    Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error)
}

type Searchable interface {
    Search(ctx context.Context, query string, opts SearchOpts) ([]SearchResult, error)
}

type Mutable interface {
    Mkdir(ctx context.Context, path string, perm Perm) error
    Remove(ctx context.Context, path string) error
    Rename(ctx context.Context, oldPath, newPath string) error
}
```

Shellfish detects capabilities at runtime via Go type assertions:

```go
if w, ok := provider.(Writable); ok {
    return w.Write(ctx, innerPath, data)
}
return ErrNotWritable
```

## Why This Matters

This design produces three important properties:

### 1. Providers only implement what they support

A read-only knowledge base implements `Provider` + `Readable` + `Searchable`. It doesn't need to stub out `Write` or `Mkdir`. The type system prevents accidental writes at compile time, and Shellfish prevents them at runtime.

```go
// A knowledge base — only navigation, reading, and search
type KnowledgeBase struct { ... }

func (kb *KnowledgeBase) Stat(ctx context.Context, path string) (*Entry, error) { ... }
func (kb *KnowledgeBase) List(ctx context.Context, path string, opts ListOpts) ([]Entry, error) { ... }
func (kb *KnowledgeBase) Open(ctx context.Context, path string) (File, error) { ... }
func (kb *KnowledgeBase) Search(ctx context.Context, query string, opts SearchOpts) ([]SearchResult, error) { ... }
```

### 2. New capabilities don't break existing providers

If Shellfish adds a `Streamable` interface tomorrow, existing providers continue to work unchanged. Only providers that want streaming implement it. No version migration needed.

### 3. Capability is self-documenting

The `mount` command shows what each mount point can do:

```
$ mount
/          memfs       (rw, exec, search, mutable)
/data      localfs     (rw, search, mutable)
/knowledge viking      (ro, search)
/tools/mcp mcp-tools   (ro, exec, search)
```

Agents can reason about what operations are available without trial and error.

## Built-in Providers

Shellfish ships with several providers:

### MemFS — In-memory filesystem

Implements: `Provider`, `Readable`, `Writable`, `Executable`, `Mutable`

The Swiss Army knife provider. Stores files and directories in memory. Supports registering Go functions as executable entries via `AddExecFunc()` — this is how builtins (`ls`, `cat`, `grep`, etc.) are implemented.

```go
fs := mounts.NewMemFS(shellfish.PermRW)
fs.AddFile("config.yaml", []byte("key: value"), shellfish.PermRO)
fs.AddExecFunc("hello", helloFunc, mounts.FuncMeta{
    Description: "Say hello",
    Usage:       "hello [name]",
})
```

### LocalFS — Host filesystem mount

Implements: `Provider`, `Readable`, `Writable`, `Searchable`, `Mutable`

Maps a host directory into Shellfish. File operations delegate to the OS. Search performs recursive text matching.

```go
fs := mounts.NewLocalFS("/home/user/projects", shellfish.PermRW)
v.Mount("/projects", fs)
// Now: cat /projects/readme.md → reads /home/user/projects/readme.md
```

### SQLiteFS — Persistent filesystem

Implements: `Provider`, `Readable`, `Writable`, `Mutable`

Stores files and metadata in a SQLite database. Useful for persisting agent memory, session logs, or any data that should survive process restarts without depending on a specific directory structure.

```go
fs, _ := mounts.NewSQLiteFS("/var/data/agent.db", shellfish.PermRW)
v.Mount("/memory", fs)
```

### MCP Providers — Bridge to MCP ecosystem

Two variants:

- **MCPToolProvider** — Exposes MCP server tools as executable entries. Each tool appears as a file under the mount point.
- **MCPResourceProvider** — Exposes MCP server resources as readable entries.

Both implement: `Provider`, `Readable`, `Searchable`, plus `Executable` (tools only)

```go
toolProvider := mounts.NewMCPToolProvider(mcpClient)
v.Mount("/tools/notion", toolProvider)
// Now: ls /tools/notion → lists available Notion tools
//      /tools/notion/search_pages "query" → executes the MCP tool
```

## The Entry Model

Every item in Shellfish is described by an `Entry`:

```go
type Entry struct {
    Name     string
    Path     string
    IsDir    bool
    Perm     Perm
    Size     int64
    MimeType string
    Modified time.Time
    Meta     map[string]string
}
```

`Perm` uses a simplified permission model: `Read`, `Write`, `Exec` flags combine into common presets (`PermRO`, `PermRW`, `PermRX`, `PermRWX`). This is intentionally simpler than Unix's user/group/other model — agents don't need that granularity.

`Meta` is a string-string map for provider-specific metadata. An MCP tool provider might include `{"tool_schema": "..."}`. A knowledge base might include `{"layer": "L0"}` to indicate context tier.

## Provider Composition

Because providers are just interfaces, they compose naturally:

- **Layered providers:** A caching provider wraps another provider, intercepting `Open` to serve from cache.
- **Filtered providers:** A permission-checking wrapper restricts which paths are accessible.
- **Aggregating providers:** A union provider merges entries from multiple underlying providers.

This is the same composition model that makes Unix powerful — small, focused components combined through standard interfaces.
