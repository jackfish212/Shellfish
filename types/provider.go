// Package types defines the core interfaces and types for AFS (Agent File System).
// This package is intentionally kept minimal with no external dependencies.
package types

import (
	"context"
	"io"
)

// Provider is the minimal interface that every mountable data source or tool
// provider implements. It supports navigation (Stat + List).
//
// Additional capabilities are expressed as optional interfaces that a Provider
// may also implement:
//   - Readable   — Open files for reading
//   - Writable   — Write / create files
//   - Executable  — Execute tools / commands
//   - Searchable  — Full-text or semantic search
//
// AFS detects these capabilities at runtime via type assertion, so providers
// only implement what they actually support.
type Provider interface {
	Stat(ctx context.Context, path string) (*Entry, error)
	List(ctx context.Context, path string, opts ListOpts) ([]Entry, error)
}

// Readable is implemented by providers that support reading file content.
type Readable interface {
	Open(ctx context.Context, path string) (File, error)
}

// Writable is implemented by providers that support creating or updating files.
type Writable interface {
	Write(ctx context.Context, path string, r io.Reader) error
}

// Executable is implemented by providers that can execute tools or commands.
type Executable interface {
	Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error)
}

// Searchable is implemented by providers that support search queries.
type Searchable interface {
	Search(ctx context.Context, query string, opts SearchOpts) ([]SearchResult, error)
}

// Mutable is implemented by providers that support namespace management
// operations (create directory, remove entry, rename/move entry).
type Mutable interface {
	Mkdir(ctx context.Context, path string, perm Perm) error
	Remove(ctx context.Context, path string) error
	Rename(ctx context.Context, oldPath, newPath string) error
}

// MountInfoProvider is implemented by providers that can describe themselves.
type MountInfoProvider interface {
	MountInfo() (name, extra string)
}
