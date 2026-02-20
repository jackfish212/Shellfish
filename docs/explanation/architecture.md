# Architecture

Shellfish has three layers: the VirtualOS orchestrator, the Shell interface, and the Provider backends. This document explains how they work together.

## VirtualOS

`VirtualOS` is the central orchestrator. It owns a `MountTable` and provides unified operations that transparently handle path resolution, mount merging, permission checking, and capability detection.

```
VirtualOS
├── MountTable          (path → Provider mapping)
├── Stat / List         (navigation)
├── Open / Write        (data access)
├── Exec                (tool execution)
├── Mkdir / Remove / Rename  (namespace mutation)
├── Search              (cross-mount search)
└── Shell()             (create shell instances)
```

All operations follow the same pattern:

1. Clean and resolve the path through the MountTable.
2. Check if the resolved Provider implements the required capability interface.
3. Check permissions on the target entry.
4. Delegate to the provider.

This means the caller never needs to know which provider handles which path — the mount table handles routing transparently.

## MountTable

The MountTable maps paths to providers using **longest-prefix matching**. When you mount a provider at `/data/projects`, any access to `/data/projects/foo/bar.md` resolves to that provider with inner path `foo/bar.md`.

Key behaviors:

- **Virtual directories.** If providers are mounted at `/data/a` and `/data/b`, then `/data` automatically exists as a virtual directory containing `a` and `b`, even if no provider is mounted at `/data` itself.

- **Mount merging.** When listing a directory, Shellfish merges entries from the resolved provider with virtual directory entries from child mounts. This means `ls /` shows both files from the root provider and mount point directories.

- **Resolution caching.** The mount table caches path-to-provider resolutions and invalidates the cache on mount/unmount operations.

Example mount layout:

```
/           → MemFS (root filesystem, standard directories)
/proc       → ProcProvider (dynamic system info)
/data       → LocalFS("/home/user/workspace")
/knowledge  → VikingProvider("http://localhost:8000")
/tools/mcp  → MCPToolProvider(notionClient)
```

An agent running `ls /` sees: `bin/  data/  knowledge/  proc/  tools/  ...`

## Shell

The Shell is Shellfish's primary interaction interface. It provides a familiar command-line environment with:

**Built-in commands** (handled directly by Shell):
- `cd`, `pwd` — navigation
- `echo` — output
- `env` — environment variables
- `history` — command history

**External commands** (resolved via PATH, executed through providers):
- `ls`, `cat`, `stat`, `write`, `mkdir`, `rm`, `mv` — filesystem operations
- `search`, `grep` — cross-mount search
- `find` — directory hierarchy search
- `head`, `tail` — partial file reading
- `mount`, `which`, `uname` — system introspection

**Composition features:**
- **Pipes:** `cat /data/log.md | grep error | head -5`
- **Redirection:** `echo "hello" > /data/note.md`, `cmd 2>&1`
- **Logical operators:** `mkdir /tmp/work && cd /tmp/work`
- **Command groups:** `{ cmd1; cmd2; } | grep pattern`
- **Here-documents:** Multi-line input via `<<EOF`
- **Environment expansion:** `$HOME`, `${VAR}`
- **Tilde expansion:** `~` resolves to user's home directory

**Command resolution** follows PATH (default: `/usr/bin:/sbin`). Commands are looked up by `Stat`-ing each candidate path and checking execute permission. This means any executable entry in any mounted provider can become a command — just ensure it's on PATH or call it by absolute path.

**Profile loading.** On creation, Shell loads `/etc/profile` and `/etc/profile.d/*.sh` to set up environment variables. User-specific history is persisted to `~/.bash_history`.

## Configure()

The `Configure()` function sets up a standard filesystem layout:

```go
rootFS, err := afs.Configure(v)
```

This:
1. Mounts a root MemFS at `/` with standard directories (`/bin`, `/usr/bin`, `/etc`, `/home`, `/tmp`, `/var`, `/proc`, etc.)
2. Mounts a `/proc` filesystem with dynamic system info (e.g., `/proc/version`)
3. Creates `/etc/profile` with default PATH

After `Configure()`, you mount your own providers and register additional builtins:

```go
builtins.RegisterBuiltinsOnFS(v, rootFS)   // adds ls, cat, write, etc.
v.Mount("/data", mounts.NewLocalFS("/workspace", afs.PermRW))
```

## Concurrency Model

Shellfish is designed for parallel-safe use:

- `MountTable` uses `sync.RWMutex` for concurrent access.
- Each `Shell` instance is independent — create one per agent session.
- `Search` fans out to all mounted Searchable providers concurrently, then merges and sorts results.
- Providers are responsible for their own internal concurrency safety.

## Data Flow

When an agent sends a shell command `cat /data/report.md | grep TODO`:

```
Agent
  │
  ▼
Shell.Execute("cat /data/report.md | grep TODO")
  │
  ├─ splitPipe → ["cat /data/report.md", "grep TODO"]
  │
  ├─ Segment 1: "cat /data/report.md"
  │   ├─ resolveCommand("cat") → /usr/bin/cat
  │   ├─ VOS.Exec("/usr/bin/cat", ["/data/report.md"])
  │   │   ├─ MountTable.Resolve("/usr/bin/cat") → MemFS, "usr/bin/cat"
  │   │   ├─ MemFS implements Executable ✓
  │   │   └─ MemFS.Exec("usr/bin/cat", ...) → runs builtinRead
  │   │       ├─ VOS.Open("/data/report.md")
  │   │       │   ├─ MountTable.Resolve("/data/report.md") → LocalFS, "report.md"
  │   │       │   └─ LocalFS.Open("report.md") → file content
  │   │       └─ returns io.ReadCloser with file content
  │   └─ output becomes pipe input
  │
  └─ Segment 2: "grep TODO"
      ├─ resolveCommand("grep") → /usr/bin/grep
      ├─ VOS.Exec("/usr/bin/grep", ["TODO"], stdin=pipe)
      └─ returns matching lines
```

The agent receives a single string result. It doesn't know or care that `/data/report.md` lives on local disk and `grep` is an in-memory Go function.
