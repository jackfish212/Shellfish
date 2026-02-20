# AFS — Agent File System

A mount-based virtual filesystem runtime for AI agents. Everything — files, tools, APIs, knowledge bases — lives under one namespace, operated through a built-in shell.

```
/
├── data/        → LocalFS (host directory)
├── memory/      → SQLiteFS (persistent storage)
├── tools/mcp/   → MCPToolProvider (external tools)
├── knowledge/   → your custom provider (semantic search, ...)
└── proc/        → ProcProvider (system info)
```

## Why

Agent frameworks give LLMs tools through JSON schemas — `read_file`, `write_file`, `search_files`, one API per operation. This works, but it's fragmented: tools don't compose, data sources have separate interfaces, and adding a new source means defining new schemas.

Unix solved this 50 years ago: **everything is a file.** Mount a data source, then `cat`, `grep`, `ls`, and pipe them together. AFS brings this to agents — one shell tool replaces dozens of individual tool definitions.

```bash
# One tool call, not five
cat /data/logs/2026-02-*.md | grep ERROR | head -10
```

## Install

```bash
go get github.com/agentfs/afs@latest
```

Requires Go 1.24+.

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/agentfs/afs"
    "github.com/agentfs/afs/builtins"
    "github.com/agentfs/afs/mounts"
)

func main() {
    v := afs.New()
    rootFS, _ := afs.Configure(v)
    builtins.RegisterBuiltinsOnFS(v, rootFS)

    v.Mount("/data", mounts.NewLocalFS("."))

    sh := v.Shell("agent")
    ctx := context.Background()

    result := sh.Execute(ctx, "ls /data")
    fmt.Print(result.Output)
}
```

## Core Concepts

### Mount anything

Every data source implements the `Provider` interface (just `Stat` + `List`) and gets mounted at a path. Additional capabilities are expressed as optional interfaces:

| Interface | Methods | Purpose |
|-----------|---------|---------|
| `Provider` | `Stat`, `List` | Navigation (required) |
| `Readable` | `Open` | Read file content |
| `Writable` | `Write` | Create/update files |
| `Executable` | `Exec` | Run tools & commands |
| `Searchable` | `Search` | Full-text or semantic search |
| `Mutable` | `Mkdir`, `Remove`, `Rename` | Namespace changes |

AFS detects capabilities at runtime via type assertions — providers only implement what they support.

### Built-in providers

| Provider | Description |
|----------|-------------|
| `MemFS` | In-memory filesystem, supports registering Go functions as commands |
| `LocalFS` | Mounts a host directory into AFS |
| `SQLiteFS` | Persistent filesystem backed by SQLite |
| `MCPToolProvider` | Bridges MCP server tools as executable entries |
| `MCPResourceProvider` | Bridges MCP server resources as readable entries |

### Shell

Agents interact through a shell with familiar commands:

```bash
ls /data                              # browse
cat /data/config.yaml                 # read
echo "done" > /memory/log.md          # write
search "auth" --scope /knowledge      # cross-mount search
cat /data/users.json | grep admin     # pipes
mkdir /tmp/work && cd /tmp/work       # logical operators
```

Built-in commands: `ls`, `cat`, `read`, `write`, `stat`, `search`, `grep`, `find`, `head`, `tail`, `mkdir`, `rm`, `mv`, `which`, `mount`, `uname`

Shell builtins: `cd`, `pwd`, `echo`, `env`, `history`

### Custom providers

Implement `Provider` + whichever capability interfaces you need:

```go
type MyProvider struct{}

func (p *MyProvider) Stat(ctx context.Context, path string) (*afs.Entry, error) { ... }
func (p *MyProvider) List(ctx context.Context, path string, opts afs.ListOpts) ([]afs.Entry, error) { ... }
func (p *MyProvider) Open(ctx context.Context, path string) (afs.File, error) { ... }
func (p *MyProvider) Search(ctx context.Context, q string, opts afs.SearchOpts) ([]afs.SearchResult, error) { ... }

v.Mount("/my-data", &MyProvider{})
```

## Integration

AFS is not an agent framework — it's the filesystem layer underneath. Multiple protocols expose the same `VirtualOS` instance:

| Protocol | Use case |
|----------|----------|
| **Go SDK** | Embed directly in Go agent code |
| **MCP Server** | Connect to OpenClaw, Claude Desktop, or any MCP-compatible agent |
| **9P Server** | Cross-language POSIX access — `mount -t 9p` and use standard file I/O from Python, Rust, etc. |

## Project Structure

```
├── vos.go              # VirtualOS orchestrator
├── mount_table.go      # Path → Provider resolution (longest-prefix matching)
├── shell.go            # Shell interface (pipes, redirects, here-docs, ...)
├── configure.go        # Standard filesystem layout + /proc
├── types/              # Core interfaces (Provider, Entry, File, Perm, ...)
├── mounts/             # Built-in providers (MemFS, LocalFS, SQLiteFS, MCP)
├── builtins/           # Shell commands (ls, cat, grep, search, ...)
└── docs/               # Documentation (Diataxis: explanation / tutorials / how-to / reference)
```

## Documentation

See [`docs/`](docs/README.md) for full documentation:

- **[Why AFS](docs/explanation/why-afs.md)** — Problem statement, positioning, comparisons
- **[Architecture](docs/explanation/architecture.md)** — VirtualOS, MountTable, Shell internals
- **[Provider Model](docs/explanation/provider-model.md)** — Capability-based interface design
- **[Shell as Interface](docs/explanation/shell-as-interface.md)** — Why shell beats tool APIs for agents
- **[Integration Strategy](docs/explanation/integration-strategy.md)** — MCP, 9P, OpenViking
- **[Getting Started](docs/tutorials/getting-started.md)** — Step-by-step tutorial
- **[Create a Provider](docs/how-to/create-provider.md)** — Build your own data source
- **[Interface Reference](docs/reference/interfaces.md)** — Complete API reference

## License

MIT
