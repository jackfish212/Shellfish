# GRASP Mount Command Implementation

## Summary

Successfully implemented a `mount` builtin command for the GRASP virtual OS, allowing users to mount different filesystem types at runtime.

## Features

### Supported Filesystem Types

1. **memfs** - In-memory filesystem
2. **localfs** - Local directory mounting
3. **githubfs** - GitHub API as filesystem
4. **unionfs** - Union/overlay filesystem

### Command Usage

```bash
# List all mount points
mount

# Mount a filesystem
mount -t <type> [options] <source> <target>

# Examples:
mount -t memfs - /mnt/memory
mount -t localfs /path/to/dir /mnt/local
mount -t githubfs - /mnt/github -o token=ghp_xxx,user=myuser
mount -t unionfs - /mnt/union -o layers=/mnt/a:/mnt/b
```

### Mount Options

- `ro` - Read-only mount
- `rw` - Read-write mount (default)
- Filesystem-specific options (e.g., `token`, `user`, `layers`)

## Implementation Details

### Files Modified/Created

1. **builtins/mount.go** - Main mount command implementation
2. **builtins/mount_test.go** - Comprehensive test suite
3. **builtins/builtins.go** - Registered mount command
4. **examples/mount_example.go** - Usage example

### Key Functions

- `builtinMount()` - Main command handler
- `listMounts()` - Display all mount points in table format
- `performMount()` - Execute mount operation with type-specific logic
- `parsePermissions()` - Parse mount options (ro/rw)

### Testing

All tests pass successfully:
- TestMount - Basic mount functionality
- TestMountMemFS - Memory filesystem mounting
- TestMountHelp - Help message display
- TestMountMissingType - Error handling for missing type
- TestMountMissingTarget - Error handling for missing target
- TestMountUnknownType - Error handling for unknown filesystem type

## Example Output

```
=== Current Mounts ===
MountID   Type        Permissions  Source
--------  ----------  -----------  ------
/         memfs       rwx          in-memory

=== Mounting memfs at /mnt/memory ===
Mounted memfs at /mnt/memory

=== All Mounts After Changes ===
MountID   Type        Permissions  Source
--------  ----------  -----------  ------
/mnt/mem  memfs       rwx          in-memory
/         memfs       rwx          in-memory
```

## Usage in Code

```go
// Create virtual OS
v := grasp.New()
root := mounts.NewMemFS(grasp.PermRW)
v.Mount("/", root)

// Register builtins (includes mount command)
builtins.RegisterBuiltinsOnFS(v, root)

// Create shell
sh := v.Shell("user")
sh.Env.Set("PATH", "/usr/bin:/bin")

// Use mount command
result := sh.Execute(ctx, "mount -t memfs - /mnt/memory")
```

## Next Steps

The mount command is fully functional and tested. Users can now:
- List all mounted filesystems
- Mount new filesystems at runtime
- Use different filesystem types with appropriate options
- Control read/write permissions for mounts
