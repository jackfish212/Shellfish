package types

import (
	"context"
	"io"
)

// File represents an open file instance. It always supports Read and Close.
// Additional capabilities (Seek, Write) can be discovered via type assertion.
type File interface {
	io.ReadCloser
	Stat() (*Entry, error)
	Name() string
}

type simpleFile struct {
	io.ReadCloser
	name  string
	entry *Entry
}

// NewFile creates a File from an io.ReadCloser and its metadata.
func NewFile(name string, entry *Entry, rc io.ReadCloser) File {
	return &simpleFile{ReadCloser: rc, name: name, entry: entry}
}

func (f *simpleFile) Stat() (*Entry, error) { return f.entry, nil }
func (f *simpleFile) Name() string          { return f.name }

type seekableFile struct {
	io.ReadCloser
	seeker io.Seeker
	name   string
	entry  *Entry
}

// NewSeekableFile creates a File that supports Seek.
func NewSeekableFile(name string, entry *Entry, rc io.ReadCloser, seeker io.Seeker) File {
	return &seekableFile{ReadCloser: rc, seeker: seeker, name: name, entry: entry}
}

func (f *seekableFile) Stat() (*Entry, error)                        { return f.entry, nil }
func (f *seekableFile) Name() string                                 { return f.name }
func (f *seekableFile) Seek(offset int64, whence int) (int64, error) { return f.seeker.Seek(offset, whence) }

// ExecutableFile is an optional interface that a File may implement to indicate
// it can be executed.
type ExecutableFile interface {
	File
	Exec(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
}

type executableFile struct {
	File
	execFn func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
}

// NewExecutableFile creates a File that also supports Exec.
func NewExecutableFile(f File, exec func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)) ExecutableFile {
	return &executableFile{File: f, execFn: exec}
}

func (f *executableFile) Exec(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
	return f.execFn(ctx, args, stdin)
}
