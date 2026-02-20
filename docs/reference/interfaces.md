# API Reference

## Provider Interfaces

Package: `github.com/agentfs/afs/types`

Re-exported at package root: `github.com/agentfs/afs`

### Provider

The base interface. Every mountable data source must implement this.

```go
type Provider interface {
    Stat(ctx context.Context, path string) (*Entry, error)
    List(ctx context.Context, path string, opts ListOpts) ([]Entry, error)
}
```

**Stat** — Returns metadata for a single entry.

- `path`: relative to mount point, `""` for mount root.
- Returns `*Entry` with at minimum `Name`, `IsDir`, `Perm` set.
- Returns error if the path does not exist.

**List** — Returns entries in a directory.

- `path`: relative to mount point, `""` for mount root.
- `opts.Recursive`: if true, list all descendants.
- Returns error if path is not a directory or does not exist.

---

### Readable

Providers that support reading file content.

```go
type Readable interface {
    Open(ctx context.Context, path string) (File, error)
}
```

**Open** — Opens a file for reading. Returns a `File` (which embeds `io.ReadCloser`). Callers must close the returned file.

---

### Writable

Providers that support creating or updating files.

```go
type Writable interface {
    Write(ctx context.Context, path string, r io.Reader) error
}
```

**Write** — Writes content from `r` to the file at `path`. Creates the file if it doesn't exist, overwrites if it does.

---

### Executable

Providers that can execute tools or commands.

```go
type Executable interface {
    Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error)
}
```

**Exec** — Executes the entry at `path` with the given arguments and optional stdin. Returns a reader for the output. This is how builtins, MCP tools, and custom commands are invoked.

---

### Searchable

Providers that support query-based retrieval.

```go
type Searchable interface {
    Search(ctx context.Context, query string, opts SearchOpts) ([]SearchResult, error)
}
```

**Search** — Performs a search within the provider's data. Semantics are provider-defined — could be full-text, regex, semantic/vector, or any other search modality.

---

### Mutable

Providers that support structural changes to the namespace.

```go
type Mutable interface {
    Mkdir(ctx context.Context, path string, perm Perm) error
    Remove(ctx context.Context, path string) error
    Rename(ctx context.Context, oldPath, newPath string) error
}
```

**Mkdir** — Creates a directory at `path` with the given permissions.

**Remove** — Removes the entry at `path`. Behavior for non-empty directories is provider-defined.

**Rename** — Moves or renames an entry. Both paths are relative to the same mount point — cross-mount renames are not supported.

---

### MountInfoProvider

Optional. Providers that can describe themselves for the `mount` command.

```go
type MountInfoProvider interface {
    MountInfo() (name, extra string)
}
```

Returns a short provider type name (e.g., `"localfs"`, `"sqlite"`, `"viking"`) and optional extra info (e.g., `"path=/workspace"`, `"3 cities"`).

---

## Core Types

### Entry

```go
type Entry struct {
    Name     string            // base name
    Path     string            // full path (set by VirtualOS)
    IsDir    bool
    Perm     Perm
    Size     int64
    MimeType string
    Modified time.Time
    Meta     map[string]string // provider-specific metadata
}

func (e Entry) String() string
```

### File

```go
type File interface {
    io.ReadCloser
    Stat() (*Entry, error)
    Name() string
}
```

Constructors:

- `NewFile(name string, entry *Entry, rc io.ReadCloser) File`
- `NewSeekableFile(name string, entry *Entry, rc io.ReadCloser, seeker io.Seeker) File`
- `NewExecutableFile(f File, exec func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)) ExecutableFile`

### ExecutableFile

```go
type ExecutableFile interface {
    File
    Exec(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
}
```

### Perm

```go
type Perm uint8

const (
    PermRead  Perm = 1 << iota // r
    PermWrite                  // w
    PermExec                   // x
)

const (
    PermNone Perm = 0
    PermRO        = PermRead               // 1
    PermRW        = PermRead | PermWrite    // 3
    PermRX        = PermRead | PermExec     // 5
    PermRWX       = PermRead | PermWrite | PermExec  // 7
)

func (p Perm) CanRead() bool
func (p Perm) CanWrite() bool
func (p Perm) CanExec() bool
func (p Perm) String() string  // e.g. "rwx", "r--", "rw-"
```

### OpenFlag

```go
type OpenFlag int

const (
    O_RDONLY OpenFlag = 0
    O_WRONLY OpenFlag = 1 << iota
    O_RDWR
    O_CREATE
    O_TRUNC
    O_APPEND
)

func (f OpenFlag) IsReadable() bool
func (f OpenFlag) IsWritable() bool
func (f OpenFlag) Has(flag OpenFlag) bool
```

### ListOpts

```go
type ListOpts struct {
    Recursive bool
}
```

### SearchOpts

```go
type SearchOpts struct {
    Scope      string // limit search to paths under this prefix
    MaxResults int
}
```

### SearchResult

```go
type SearchResult struct {
    Entry   Entry
    Score   float64
    Snippet string
}
```

---

## Errors

```go
var (
    ErrNotFound        = errors.New("afs: not found")
    ErrNotExecutable   = errors.New("afs: not executable")
    ErrNotReadable     = errors.New("afs: permission denied: not readable")
    ErrNotWritable     = errors.New("afs: permission denied: not writable")
    ErrIsDir           = errors.New("afs: is a directory")
    ErrNotDir          = errors.New("afs: not a directory")
    ErrAlreadyMounted  = errors.New("afs: path already mounted")
    ErrMountUnderMount = errors.New("afs: mount under existing mount point")
    ErrNotSupported    = errors.New("afs: operation not supported")
    ErrParentNotExist  = errors.New("afs: parent directory does not exist")
)
```

---

## VirtualOS

Package: `github.com/agentfs/afs`

```go
func New() *VirtualOS

func (v *VirtualOS) Mount(path string, p Provider) error
func (v *VirtualOS) Unmount(path string) error
func (v *VirtualOS) MountTable() *MountTable

func (v *VirtualOS) Stat(ctx context.Context, path string) (*Entry, error)
func (v *VirtualOS) List(ctx context.Context, path string, opts ListOpts) ([]Entry, error)
func (v *VirtualOS) Open(ctx context.Context, path string) (File, error)
func (v *VirtualOS) OpenFile(ctx context.Context, path string, flag OpenFlag) (File, error)
func (v *VirtualOS) Write(ctx context.Context, path string, reader io.Reader) error
func (v *VirtualOS) Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error)
func (v *VirtualOS) Mkdir(ctx context.Context, path string, perm Perm) error
func (v *VirtualOS) Remove(ctx context.Context, path string) error
func (v *VirtualOS) Rename(ctx context.Context, oldPath, newPath string) error
func (v *VirtualOS) Search(ctx context.Context, query string, opts SearchOpts) ([]SearchResult, error)
func (v *VirtualOS) Shell(user string) *Shell
```

---

## MountTable

```go
func (t *MountTable) Mount(mountPath string, p Provider) error
func (t *MountTable) Unmount(mountPath string) error
func (t *MountTable) Resolve(fullPath string) (Provider, string, error)
func (t *MountTable) ChildMounts(dirPath string) []Entry
func (t *MountTable) All() []string
func (t *MountTable) AllInfo() []MountInfo

type MountInfo struct {
    Path        string
    Provider    Provider
    Permissions string
}
```

---

## Shell

Package: `github.com/agentfs/afs/shell`

Re-exported at: `github.com/agentfs/afs`

```go
func NewShell(v VirtualOS, user string) *Shell

func (s *Shell) Execute(ctx context.Context, cmdLine string) *ExecResult
func (s *Shell) Cwd() string
func (s *Shell) History() []string
func (s *Shell) ClearHistory()
func (s *Shell) HistorySize() int

type ExecResult struct {
    Output string
    Code   int
}

type ShellEnv struct { /* ... */ }

func (e *ShellEnv) Get(key string) string
func (e *ShellEnv) Set(key, value string)
func (e *ShellEnv) All() map[string]string

const MaxHistorySize = 1000
```

---

## Setup Functions

```go
func Configure(v *VirtualOS) (*mounts.MemFS, error)
func MountRootFS(v *VirtualOS) (*mounts.MemFS, error)
func MountProc(v *VirtualOS) error
func GetVersionInfo() VersionInfo

type VersionInfo struct {
    Version   string
    BuildDate string
    GitCommit string
    GoVersion string
    Platform  string
}

func (v VersionInfo) ProcVersion() string
```

---

## Providers

Package: `github.com/agentfs/afs/mounts`

### MemFS

```go
func NewMemFS(perm Perm) *MemFS

func (fs *MemFS) AddFile(path string, content []byte, perm Perm)
func (fs *MemFS) AddDir(path string)
func (fs *MemFS) AddFunc(path string, fn Func, meta FuncMeta)
func (fs *MemFS) AddExecFunc(path string, fn ExecFunc, meta FuncMeta)
func (fs *MemFS) RemoveFunc(path string) bool

// Implements: Provider, Readable, Writable, Executable, Mutable, MountInfoProvider
```

### LocalFS

```go
func NewLocalFS(root string, perm Perm) *LocalFS

// Implements: Provider, Readable, Writable, Searchable, Mutable, MountInfoProvider
```

### SQLiteFS

```go
func NewSQLiteFS(dbPath string, perm Perm) (*SQLiteFS, error)

func (fs *SQLiteFS) Close() error

// Implements: Provider, Readable, Writable, Mutable, MountInfoProvider
```

### MCP Providers

```go
func NewMCPToolProvider(client MCPClient) *MCPToolProvider
func NewMCPResourceProvider(client MCPClient) *MCPResourceProvider
func MountMCP(v interface{ Mount(string, Provider) error }, basePath string, client MCPClient) error

// MCPToolProvider implements: Provider, Readable, Executable, Searchable, MountInfoProvider
// MCPResourceProvider implements: Provider, Readable, Searchable, MountInfoProvider

type MCPClient interface {
    ListTools(ctx context.Context) ([]MCPTool, error)
    CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error)
    ListResources(ctx context.Context) ([]MCPResource, error)
    ReadResource(ctx context.Context, uri string) (string, error)
    ListPrompts(ctx context.Context) ([]MCPPrompt, error)
    GetPrompt(ctx context.Context, name string, args map[string]any) (string, error)
}
```

### Function Types

```go
type Func func(ctx context.Context, args []string, stdin string) (string, error)
type ExecFunc func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)

type FuncMeta struct {
    Description string
    Usage       string
}
```

---

## Builtins

Package: `github.com/agentfs/afs/builtins`

```go
func RegisterBuiltins(v *afs.VirtualOS, mountPath string) error
func RegisterBuiltinsOnFS(v *afs.VirtualOS, fs *mounts.MemFS) error
```

Registered commands (at `/usr/bin/`):

| Command | Description | Key flags |
|---------|-------------|-----------|
| `ls` | List directory entries | `-l` (long), `-a` (all) |
| `cat` | Read file content (reads stdin if no file) | — |
| `read` | Read file content | — |
| `write` | Write stdin or args to file | — |
| `stat` | Show entry metadata | — |
| `search` | Cross-mount search | `--scope`, `--max` |
| `grep` | Filter lines matching pattern (from stdin) | — |
| `find` | Search directory hierarchy | `-name`, `-type`, `-maxdepth`, `-mindepth`, `-path` |
| `head` | Output first part of file | `-n` (lines), `-c` (bytes) |
| `tail` | Output last part of file | `-n` (lines), `-c` (bytes) |
| `mkdir` | Create directories | `-p` (parents) |
| `rm` | Remove files/directories | `-r` (recursive), `-f` (force) |
| `mv` | Move/rename files | — |
| `which` | Show full path of command | — |
| `mount` | List mount points with permissions | — |
| `uname` | Print system information | `-a`, `-s`, `-n`, `-r`, `-v`, `-m` |

All commands support `-h` / `--help` for usage information.

---

## Context Helpers

```go
func WithEnv(ctx context.Context, env map[string]string) context.Context
func Env(ctx context.Context, key string) string
func CleanPath(p string) string
```
