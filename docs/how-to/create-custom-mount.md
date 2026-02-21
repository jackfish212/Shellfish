# Mount Plugin System

The GRASP mount system supports a plugin architecture that allows third-party libraries to register custom filesystem types.

## Overview

The mount plugin system provides a simple API for registering new filesystem types that can be mounted using the `mount` command. This allows you to extend GRASP with support for cloud storage, remote filesystems, databases, or any other data source that can be represented as a filesystem.

## Built-in Filesystem Types

GRASP comes with several built-in filesystem types:

- **memfs**: In-memory filesystem
- **localfs**: Mount a local directory
- **sqlitefs**: Mount a SQLite database as filesystem
- **githubfs**: Mount GitHub API as filesystem
- **unionfs**: Union/overlay filesystem

## Registering Custom Filesystem Types

To register a custom filesystem type, use the `RegisterMountType` function:

```go
import (
    "context"
    "github.com/jackfish212/grasp"
    "github.com/jackfish212/grasp/builtins"
)

func init() {
    err := builtins.RegisterMountType(builtins.MountTypeInfo{
        Name:        "s3fs",
        Description: "Mount an S3 bucket as filesystem",
        Usage:       "mount -t s3fs s3://bucket /mnt/s3 -o region=us-east-1",
        Handler: func(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error {
            // Create your filesystem implementation
            fs := NewS3FS(source, opts)

            // Mount it
            return v.Mount(target, fs)
        },
    })
    if err != nil {
        panic(err)
    }
}
```

## MountTypeInfo Structure

The `MountTypeInfo` struct has the following fields:

- **Name** (string): The filesystem type name (e.g., "s3fs", "httpfs")
- **Description** (string): A short description shown in help text
- **Usage** (string): An example usage command
- **Handler** (MountHandler): The function that performs the mount

## MountHandler Function

The `MountHandler` function signature is:

```go
type MountHandler func(
    ctx context.Context,
    v *grasp.VirtualOS,
    source string,
    target string,
    opts map[string]string,
) error
```

Parameters:
- **ctx**: Context for cancellation and timeouts
- **v**: The VirtualOS instance to mount into
- **source**: The source path/URL from the mount command
- **target**: The target mount point path
- **opts**: Parsed mount options from `-o` flag

## Implementing a Custom Filesystem

Your filesystem must implement the `types.Provider` interface:

```go
type Provider interface {
    fs.FS                    // Open, Stat, ReadDir
    Create(name string) (io.WriteCloser, error)
    Remove(name string) error
    Mkdir(name string, perm fs.FileMode) error
}
```

Optionally, implement `MountInfoProvider` for better mount listing:

```go
type MountInfoProvider interface {
    MountInfo() (typ string, extra string)
}
```

## Example: S3 Filesystem

```go
package main

import (
    "context"
    "fmt"
    "io/fs"

    "github.com/jackfish212/grasp"
    "github.com/jackfish212/grasp/builtins"
    "github.com/jackfish212/grasp/types"
)

type S3FS struct {
    bucket string
    region string
    perm   types.Perm
}

func NewS3FS(bucket, region string, perm types.Perm) *S3FS {
    return &S3FS{bucket: bucket, region: region, perm: perm}
}

func (s *S3FS) Open(name string) (fs.File, error) {
    // Implement S3 file opening
}

func (s *S3FS) Stat(name string) (fs.FileInfo, error) {
    // Implement S3 stat
}

func (s *S3FS) ReadDir(name string) ([]fs.DirEntry, error) {
    // Implement S3 directory listing
}

func (s *S3FS) Create(name string) (io.WriteCloser, error) {
    // Implement S3 file creation
}

func (s *S3FS) Remove(name string) error {
    // Implement S3 file deletion
}

func (s *S3FS) Mkdir(name string, perm fs.FileMode) error {
    // Implement S3 directory creation
}

func (s *S3FS) MountInfo() (string, string) {
    return "s3fs", fmt.Sprintf("s3://%s (%s)", s.bucket, s.region)
}

func init() {
    builtins.RegisterMountType(builtins.MountTypeInfo{
        Name:        "s3fs",
        Description: "Mount an S3 bucket as filesystem",
        Usage:       "mount -t s3fs s3://bucket /mnt/s3 -o region=us-east-1",
        Handler: func(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error {
            bucket := source[5:] // strip "s3://"
            region := opts["region"]
            if region == "" {
                region = "us-east-1"
            }

            perm := types.PermRW
            if opts["ro"] == "true" {
                perm = types.PermRO
            }

            fs := NewS3FS(bucket, region, perm)
            return v.Mount(target, fs)
        },
    })
}
```

## Usage

Once registered, users can mount your filesystem type:

```bash
# Mount an S3 bucket
mount -t s3fs s3://my-bucket /mnt/s3 -o region=us-west-2

# List all mounts
mount

# The custom filesystem appears in the list
```

## API Functions

### RegisterMountType

```go
func RegisterMountType(info MountTypeInfo) error
```

Registers a new filesystem type. Returns an error if the type name is empty, the handler is nil, or the type is already registered.

### UnregisterMountType

```go
func UnregisterMountType(name string)
```

Removes a filesystem type from the registry.

### GetMountType

```go
func GetMountType(name string) (*MountTypeInfo, bool)
```

Returns information about a registered filesystem type.

### ListMountTypes

```go
func ListMountTypes() []*MountTypeInfo
```

Returns all registered filesystem types.

## Best Practices

1. **Register in init()**: Register your filesystem types in an `init()` function so they're available when your package is imported.

2. **Validate options**: Check that required options are provided and return clear error messages.

3. **Handle permissions**: Respect the `ro` and `rw` options from the mount command.

4. **Implement MountInfo**: Provide useful information for the mount listing.

5. **Context handling**: Respect the context for cancellation and timeouts.

6. **Error messages**: Return descriptive errors that help users fix issues.

## Thread Safety

The mount registry is thread-safe and can be accessed concurrently from multiple goroutines.

## See Also

- [examples/custom-mount/main.go](../../examples/custom-mount/main.go) - Complete example with S3 and HTTP filesystems
- [examples/custom-mount/simple_example.go](../../examples/custom-mount/simple_example.go) - Simple mount example
- [builtins/mount_registry.go](../../builtins/mount_registry.go) - Registry implementation
- [builtins/mount.go](../../builtins/mount.go) - Mount command implementation
