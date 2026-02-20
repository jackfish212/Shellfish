# Provider Interfaces Reference

Package: `github.com/agentfs/afs/types`

Re-exported at package root: `github.com/agentfs/afs`

## Provider

The base interface. Every mountable data source must implement this.

```go
type Provider interface {
    Stat(ctx context.Context, path string) (*Entry, error)
    List(ctx context.Context, path string, opts ListOpts) ([]Entry, error)
}
```

### Stat

Returns metadata for a single entry.

- `path`: relative to mount point, `""` for mount root.
- Returns `*Entry` with at minimum `Name`, `IsDir`, `Perm` set.
- Returns error if the path does not exist.

### List

Returns entries in a directory.

- `path`: relative to mount point, `""` for mount root.
- `opts.Recursive`: if true, list all descendants.
- Returns error if path is not a directory or does not exist.

---

## Readable

Providers that support reading file content.

```go
type Readable interface {
    Open(ctx context.Context, path string) (File, error)
}
```

### Open

Opens a file for reading. Returns a `File` (which embeds `io.ReadCloser`). Callers must close the returned file.

---

## Writable

Providers that support creating or updating files.

```go
type Writable interface {
    Write(ctx context.Context, path string, r io.Reader) error
}
```

### Write

Writes content from `r` to the file at `path`. Creates the file if it doesn't exist, overwrites if it does.

---

## Executable

Providers that can execute tools or commands.

```go
type Executable interface {
    Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error)
}
```

### Exec

Executes the entry at `path` with the given arguments and optional stdin. Returns a reader for the output. This is how builtins, MCP tools, and custom commands are invoked.

---

## Searchable

Providers that support query-based retrieval.

```go
type Searchable interface {
    Search(ctx context.Context, query string, opts SearchOpts) ([]SearchResult, error)
}
```

### Search

Performs a search within the provider's data. Semantics are provider-defined — could be full-text, regex, semantic/vector, or any other search modality.

---

## Mutable

Providers that support structural changes to the namespace.

```go
type Mutable interface {
    Mkdir(ctx context.Context, path string, perm Perm) error
    Remove(ctx context.Context, path string) error
    Rename(ctx context.Context, oldPath, newPath string) error
}
```

### Mkdir

Creates a directory at `path` with the given permissions.

### Remove

Removes the entry at `path`. Behavior for non-empty directories is provider-defined.

### Rename

Moves or renames an entry. Both paths are relative to the same mount point — cross-mount renames are not supported.

---

## MountInfoProvider

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
- `NewSeekableFile(name string, entry *Entry, rs io.ReadSeeker) File`
- `NewExecutableFile(name string, entry *Entry, rc io.ReadCloser) File`

### Perm

```go
type Perm uint8

const (
    PermNone  Perm = 0
    PermRead  Perm = 1 << 0  // 1
    PermWrite Perm = 1 << 1  // 2
    PermExec  Perm = 1 << 2  // 4
)

const (
    PermRO  = PermRead               // 1
    PermRW  = PermRead | PermWrite    // 3
    PermRX  = PermRead | PermExec     // 5
    PermRWX = PermRead | PermWrite | PermExec  // 7
)

func (p Perm) CanRead() bool
func (p Perm) CanWrite() bool
func (p Perm) CanExec() bool
func (p Perm) String() string  // e.g. "rwx", "r--", "rw-"
```

### OpenFlag

```go
type OpenFlag uint8

const (
    O_RDONLY OpenFlag = 0
    O_WRONLY OpenFlag = 1
    O_RDWR   OpenFlag = 2
    O_CREATE OpenFlag = 4
    O_TRUNC  OpenFlag = 8
    O_APPEND OpenFlag = 16
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

## VirtualOS API

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

## Shell API

```go
func NewShell(v *VirtualOS, user string) *Shell

func (s *Shell) Execute(ctx context.Context, cmdLine string) *ExecResult
func (s *Shell) Cwd() string

type ExecResult struct {
    Output string
    Code   int
}
```

## Setup Functions

```go
func Configure(v *VirtualOS) (*mounts.MemFS, error)
func MountRootFS(v *VirtualOS) (*mounts.MemFS, error)
func MountProc(v *VirtualOS) error
```
