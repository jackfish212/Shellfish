package afs

import (
	"bytes"
	"context"
	"fmt"
)

// writableFile implements File for write-mode opens.
type writableFile struct {
	name   string
	inner  string
	w      Writable
	flag   OpenFlag
	buf    bytes.Buffer
	closed bool
}

func newWritableFile(name, inner string, w Writable, flag OpenFlag) *writableFile {
	return &writableFile{name: name, inner: inner, w: w, flag: flag}
}

func (f *writableFile) Read(p []byte) (int, error) {
	return 0, fmt.Errorf("%w: file opened for writing only", ErrNotReadable)
}

func (f *writableFile) Write(p []byte) (int, error) {
	if f.closed {
		return 0, fmt.Errorf("write on closed file: %s", f.name)
	}
	return f.buf.Write(p)
}

func (f *writableFile) Close() error {
	if f.closed {
		return nil
	}
	f.closed = true
	ctx := context.Background()
	return f.w.Write(ctx, f.inner, &f.buf)
}

func (f *writableFile) Stat() (*Entry, error) {
	return &Entry{
		Name: baseName(f.name),
		Path: f.name,
		Perm: PermRW,
		Size: int64(f.buf.Len()),
	}, nil
}

func (f *writableFile) Name() string { return f.name }
