package builtins

import (
	"context"
	"fmt"
	"strings"
	"sync"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

// MountHandler is a function that creates and mounts a filesystem.
// It receives the VirtualOS, source path, target path, and mount options.
// It should return an error if the mount fails.
type MountHandler func(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error

// MountTypeInfo describes a filesystem type that can be mounted.
type MountTypeInfo struct {
	Name        string       // Filesystem type name (e.g., "memfs", "s3fs")
	Description string       // Short description
	Usage       string       // Usage example
	Handler     MountHandler // Function to perform the mount
}

var (
	mountRegistry = make(map[string]*MountTypeInfo)
	registryMu    sync.RWMutex
)

// RegisterMountType registers a new filesystem type that can be mounted.
// This allows third-party libraries to add support for custom filesystems.
//
// Example:
//   builtins.RegisterMountType(builtins.MountTypeInfo{
//       Name:        "s3fs",
//       Description: "Mount an S3 bucket as filesystem",
//       Usage:       "mount -t s3fs s3://bucket /mnt/s3 -o region=us-east-1,key=xxx",
//       Handler: func(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error {
//           // Create and mount your custom filesystem
//           fs := s3fs.New(source, opts)
//           return v.Mount(target, fs)
//       },
//   })
func RegisterMountType(info MountTypeInfo) error {
	registryMu.Lock()
	defer registryMu.Unlock()

	if info.Name == "" {
		return fmt.Errorf("mount type name cannot be empty")
	}
	if info.Handler == nil {
		return fmt.Errorf("mount handler cannot be nil")
	}
	if _, exists := mountRegistry[info.Name]; exists {
		return fmt.Errorf("mount type %q already registered", info.Name)
	}

	mountRegistry[info.Name] = &info
	return nil
}

// UnregisterMountType removes a filesystem type from the registry.
func UnregisterMountType(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(mountRegistry, name)
}

// GetMountType returns information about a registered filesystem type.
func GetMountType(name string) (*MountTypeInfo, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	info, ok := mountRegistry[name]
	return info, ok
}

// ListMountTypes returns all registered filesystem types.
func ListMountTypes() []*MountTypeInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()

	types := make([]*MountTypeInfo, 0, len(mountRegistry))
	for _, info := range mountRegistry {
		types = append(types, info)
	}
	return types
}

// Built-in mount handlers

func mountMemFS(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error {
	perm := parsePermissions(opts)
	fs := mounts.NewMemFS(perm)
	return v.Mount(target, fs)
}

func mountLocalFS(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error {
	if source == "" || source == "-" {
		return fmt.Errorf("localfs requires a source directory")
	}
	perm := parsePermissions(opts)
	fs := mounts.NewLocalFS(source, perm)
	return v.Mount(target, fs)
}

func mountSQLiteFS(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error {
	if source == "" || source == "-" {
		return fmt.Errorf("sqlitefs requires a database path")
	}
	perm := parsePermissions(opts)
	fs, err := mounts.NewSQLiteFS(source, perm)
	if err != nil {
		return fmt.Errorf("failed to create sqlitefs: %w", err)
	}
	return v.Mount(target, fs)
}

func mountGitHubFS(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error {
	token := opts["token"]
	user := opts["user"]
	if token == "" || user == "" {
		return fmt.Errorf("githubfs requires token and user options")
	}
	perm := parsePermissions(opts)
	var ghOpts []mounts.GitHubFSOption
	if token != "" {
		ghOpts = append(ghOpts, mounts.WithGitHubToken(token))
	}
	if user != "" {
		ghOpts = append(ghOpts, mounts.WithGitHubUser(user))
	}
	// Apply permissions if needed (depends on GitHubFS implementation)
	_ = perm
	fs := mounts.NewGitHubFS(ghOpts...)
	return v.Mount(target, fs)
}

func mountUnionFS(ctx context.Context, v *grasp.VirtualOS, source, target string, opts map[string]string) error {
	layersStr := opts["layers"]
	if layersStr == "" {
		return fmt.Errorf("unionfs requires layers option")
	}
	layerPaths := strings.Split(layersStr, ":")
	var layers []mounts.Layer
	for _, path := range layerPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		// Resolve the path to get the provider
		p, inner, err := v.MountTable().Resolve(path)
		if err != nil {
			return fmt.Errorf("layer path %s not found: %w", path, err)
		}
		if inner != "" {
			return fmt.Errorf("layer path %s must be a mount point", path)
		}
		layers = append(layers, mounts.Layer{Provider: p})
	}
	if len(layers) == 0 {
		return fmt.Errorf("unionfs requires at least one layer")
	}
	fs := mounts.NewUnion(layers...)
	return v.Mount(target, fs)
}

// init registers built-in filesystem types
func init() {
	// Register built-in types
	RegisterMountType(MountTypeInfo{
		Name:        "memfs",
		Description: "Mount an in-memory filesystem",
		Usage:       "mount -t memfs - /mnt/mem -o rw",
		Handler:     mountMemFS,
	})

	RegisterMountType(MountTypeInfo{
		Name:        "localfs",
		Description: "Mount a local directory",
		Usage:       "mount -t localfs /path/to/dir /mnt/local -o ro",
		Handler:     mountLocalFS,
	})

	RegisterMountType(MountTypeInfo{
		Name:        "sqlitefs",
		Description: "Mount a SQLite database as filesystem",
		Usage:       "mount -t sqlitefs /path/to/db.sqlite /mnt/db -o rw",
		Handler:     mountSQLiteFS,
	})

	RegisterMountType(MountTypeInfo{
		Name:        "githubfs",
		Description: "Mount GitHub API as filesystem",
		Usage:       "mount -t githubfs - /mnt/github -o token=ghp_xxx,user=myuser",
		Handler:     mountGitHubFS,
	})

	RegisterMountType(MountTypeInfo{
		Name:        "unionfs",
		Description: "Mount a union filesystem (overlay)",
		Usage:       "mount -t unionfs - /mnt/union -o layers=/mnt/a:/mnt/b",
		Handler:     mountUnionFS,
	})
}
