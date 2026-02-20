# Integration Strategy

Shellfish is designed to be used from any language, any agent framework, through multiple protocols. This document explains the integration layers and how they connect Shellfish to the broader ecosystem.

## Overview

```
┌───────────────────────────────────────────────────────────┐
│                      Agent Frameworks                      │
│                                                           │
│  OpenClaw    Claude    PicoClaw    Custom    Python Agent  │
│  (TS)       Desktop    (Go)       (Go)      (Python)      │
│     │          │         │          │           │          │
│     ▼          ▼         │          │           ▼          │
│  ┌──────┐  ┌──────┐     │          │       ┌──────┐       │
│  │ MCP  │  │ MCP  │     │          │       │  9P  │       │
│  └──┬───┘  └──┬───┘     │          │       └──┬───┘       │
├─────┼─────────┼──────────┼──────────┼──────────┼──────────┤
│     │         │          │          │          │           │
│     └─────────┴──────────┼──────────┘          │           │
│                          ▼                     ▼           │
│                    ┌───────────────────────────────┐       │
│                    │         VirtualOS             │       │
│                    │         + Shell               │       │
│                    │         + MountTable          │       │
│                    └───────────┬───────────────────┘       │
│                               │                           │
│               ┌───────┬───────┼───────┬──────┐            │
│               ▼       ▼       ▼       ▼      ▼            │
│            MemFS  LocalFS  SQLiteFS  MCP  Viking          │
│                                     Provider Provider     │
└───────────────────────────────────────────────────────────┘
```

## Layer 1: Go SDK (Direct Embedding)

The most direct integration. Any Go program imports Shellfish as a library:

```go
import (
    "github.com/jackfish212/shellfish"
    "github.com/jackfish212/shellfish/builtins"
    "github.com/jackfish212/shellfish/mounts"
)

v := shellfish.New()
rootFS, _ := shellfish.Configure(v)
builtins.RegisterBuiltinsOnFS(v, rootFS)

v.Mount("/data", mounts.NewLocalFS("/workspace", shellfish.PermRW))

sh := v.Shell("agent")
result := sh.Execute(ctx, "ls /data")
fmt.Println(result.Output)
```

**Target audience:** Go-based agent frameworks (PicoClaw, custom agents), applications that want an embedded virtual filesystem.

**Advantages:** Zero overhead, full type safety, direct access to Provider interfaces for advanced use cases.

## Layer 2: MCP Server

[MCP (Model Context Protocol)](https://modelcontextprotocol.io) is the emerging standard for connecting agents to external tools. Shellfish exposes itself as an MCP server, making it accessible to any MCP-compatible agent — including OpenClaw, Claude Desktop, and the OpenAI Agents SDK.

### Tools

The MCP server exposes Shellfish operations as tools:

| Tool | Description |
|------|-------------|
| `shellfish_shell` | Execute a shell command (the primary interface) |
| `shellfish_read` | Read a file (convenience shortcut) |
| `shellfish_write` | Write to a file |
| `shellfish_search` | Cross-mount search |
| `shellfish_mount` | List or manage mount points |

The `shellfish_shell` tool is the most important — it provides access to the full shell with pipes, redirects, and all builtins through a single tool call.

### Resources

Shellfish also exposes content through MCP Resources:

```
shellfish://mounts         → current mount table
shellfish://tree/{path}    → directory tree at path
shellfish://file/{path}    → file content
```

This allows MCP clients to browse Shellfish content without tool calls — useful for context injection.

### Integration with OpenClaw

OpenClaw supports MCP servers through its plugin system. Shellfish connects as a stdio-based MCP server:

```json
{
  "mcpServers": {
    "shellfish": {
      "command": "shellfish-server",
      "args": [
        "--mount", "/data:./workspace",
        "--mount", "/docs:./documentation"
      ],
      "transport": "stdio"
    }
  }
}
```

Once connected, OpenClaw's agent can use Shellfish through natural shell commands:

```
> shellfish_shell "ls /data"
> shellfish_shell "cat /docs/api-guide.md | grep authentication"
> shellfish_shell "search 'error handling' --scope /data"
```

### Integration with Claude Desktop

Claude Desktop natively supports MCP servers. Add Shellfish to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "shellfish": {
      "command": "shellfish-server",
      "args": ["--mount", "/projects:~/projects"]
    }
  }
}
```

## Layer 3: 9P Server

[9P](https://en.wikipedia.org/wiki/9P_(protocol)) is Plan 9's file protocol — a minimal, well-defined protocol for accessing remote filesystems. It's the ideal cross-language bridge for Shellfish because:

1. **True POSIX semantics.** Any language can `open()`, `read()`, `write()` files over 9P.
2. **Kernel-level mounting.** On Linux: `mount -t 9p localhost /mnt/shellfish`. The filesystem appears natively.
3. **Minimal protocol.** ~13 message types. Implementations exist in Go, Python, Rust, C, Java.
4. **No code generation.** Unlike gRPC, clients don't need generated stubs.

### Usage

Start the 9P server:

```bash
shellfish-server --9p :5640 --mount /data:./workspace
```

Mount it on Linux:

```bash
mount -t 9p -o port=5640,trans=tcp localhost /mnt/shellfish
```

Now any program — Python, Rust, shell scripts — accesses Shellfish through standard file I/O:

```python
# Python agent accessing Shellfish
with open("/mnt/shellfish/data/report.md") as f:
    content = f.read()

os.listdir("/mnt/shellfish/tools/")
```

```bash
# Shell script
cat /mnt/shellfish/data/config.yaml | grep database
```

### 9P vs. gRPC vs. REST

| Aspect | 9P | gRPC | REST |
|--------|-----|------|------|
| Client code needed | None (OS mount) | Generated stubs | HTTP client |
| Semantics | File I/O (open/read/write) | RPC calls | HTTP verbs |
| Streaming | Native (file reads) | Bidirectional streams | SSE/WebSocket |
| Cross-language | Any (OS-level) | Per-language codegen | HTTP |
| Overhead | Minimal | Moderate (protobuf) | High (JSON) |

9P gives the broadest reach with the least friction. A Python agent doesn't need a Shellfish SDK — it just reads files.

## OpenViking Integration

[OpenViking](https://github.com/volcengine/OpenViking) is ByteDance's open-source context database for AI agents. It uses a `viking://` URI scheme and provides L0/L1/L2 tiered context loading with semantic retrieval.

Shellfish integrates with OpenViking by implementing a `VikingProvider` — a Shellfish provider that connects to OpenViking's HTTP server and maps its operations to the Provider interface.

### Mapping

```
Shellfish Path                    OpenViking API
──────────────────                ──────────────────
ls /ctx/resources/                → client.ls("viking://resources/")
cat /ctx/resources/doc.md         → client.read("viking://resources/doc.md")
cat /ctx/resources/.abstract      → client.abstract("viking://resources/")
cat /ctx/resources/.overview      → client.overview("viking://resources/")
search "query" /ctx/              → client.find("query", target_uri="viking://")
```

**Key design: L0/L1 as virtual files.** Every directory under the Viking mount automatically exposes `.abstract` (~100 tokens) and `.overview` (~2K tokens) virtual files. Agents read them with `cat` to decide whether to load the full L2 content — no special API needed.

### Value for both sides

**For Shellfish:** Gains semantic retrieval, automatic summarization, and the L0/L1/L2 tiered loading model — capabilities that a pure filesystem can't provide on its own.

**For OpenViking:** Gains a shell interface, pipe composition, unified namespace with other data sources, and cross-language access through 9P. An agent can `grep` through OpenViking content, pipe search results to other commands, and mix OpenViking data with local files in a single command.

### Example session

```bash
$ ls /ctx/resources/
my_project/  documentation/  codebase/

$ cat /ctx/resources/my_project/.abstract
A backend service implementing user auth, payment processing, and notification.

$ cat /ctx/resources/my_project/.overview
# my_project
## Structure
- src/auth/ — JWT-based authentication
- src/payments/ — Stripe integration
- src/notifications/ — Email and push notifications
## Key decisions
- Chose JWT over session cookies for stateless scaling
...

$ search "payment error handling" --scope /ctx/
/ctx/resources/my_project/src/payments/stripe.py (score: 0.89)
/ctx/resources/my_project/docs/error-codes.md (score: 0.72)

$ cat /ctx/resources/my_project/src/payments/stripe.py | grep "except"
```

The agent traverses from abstract → overview → specific file without ever leaving the shell.

## `shellfish-server`: The Unified Binary

The `shellfish-server` command starts Shellfish with one or more protocol adapters:

```bash
shellfish-server \
  --mcp stdio \                    # MCP over stdin/stdout
  --9p :5640 \                     # 9P on TCP port 5640
  --mount /data:./workspace \      # mount local directory
  --mount /ctx:viking:http://localhost:8000  # mount OpenViking
```

A single process serves multiple protocols simultaneously. The MCP adapter and 9P adapter share the same `VirtualOS` instance, so they see the same mount table and the same data.
