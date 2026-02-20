package mounts

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentfs/afs/types"
)

var (
	_ types.Provider   = (*LocalFS)(nil)
	_ types.Readable   = (*LocalFS)(nil)
	_ types.Writable   = (*LocalFS)(nil)
	_ types.Searchable = (*LocalFS)(nil)
	_ types.Mutable    = (*LocalFS)(nil)
)

// LocalFS mounts a host directory into AFS.
type LocalFS struct {
	root string
	perm types.Perm
}

func NewLocalFS(root string, perm types.Perm) *LocalFS {
	return &LocalFS{root: filepath.Clean(root), perm: perm}
}

func (fs *LocalFS) hostPath(vosPath string) string {
	if vosPath == "" {
		return fs.root
	}
	return filepath.Join(fs.root, filepath.FromSlash(vosPath))
}

func (fs *LocalFS) Stat(_ context.Context, path string) (*types.Entry, error) {
	hp := fs.hostPath(path)
	info, err := os.Stat(hp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
		}
		return nil, err
	}
	return fs.infoToEntry(path, info), nil
}

func (fs *LocalFS) List(_ context.Context, path string, _ types.ListOpts) ([]types.Entry, error) {
	hp := fs.hostPath(path)
	dirEntries, err := os.ReadDir(hp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
		}
		return nil, err
	}

	entries := make([]types.Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		info, infoErr := de.Info()
		if infoErr != nil {
			continue
		}
		childPath := de.Name()
		if path != "" {
			childPath = path + "/" + de.Name()
		}
		entries = append(entries, *fs.infoToEntry(childPath, info))
	}
	return entries, nil
}

func (fs *LocalFS) Open(_ context.Context, path string) (types.File, error) {
	if !fs.perm.CanRead() {
		return nil, fmt.Errorf("%w: %s", types.ErrNotReadable, path)
	}
	hp := fs.hostPath(path)
	f, err := os.Open(hp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
		}
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	entry := fs.infoToEntry(path, info)
	return types.NewSeekableFile(path, entry, f, f), nil
}

func (fs *LocalFS) Write(_ context.Context, path string, r io.Reader) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	hp := fs.hostPath(path)
	if err := os.MkdirAll(filepath.Dir(hp), 0o755); err != nil {
		return err
	}
	f, err := os.Create(hp)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (fs *LocalFS) Mkdir(_ context.Context, path string, _ types.Perm) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	hp := fs.hostPath(path)
	return os.MkdirAll(hp, 0o755)
}

func (fs *LocalFS) Remove(_ context.Context, path string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}
	hp := fs.hostPath(path)
	if _, err := os.Stat(hp); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	return os.RemoveAll(hp)
}

func (fs *LocalFS) Rename(_ context.Context, oldPath, newPath string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, oldPath)
	}
	hpOld := fs.hostPath(oldPath)
	hpNew := fs.hostPath(newPath)
	if _, err := os.Stat(hpOld); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", types.ErrNotFound, oldPath)
	}
	if err := os.MkdirAll(filepath.Dir(hpNew), 0o755); err != nil {
		return err
	}
	return os.Rename(hpOld, hpNew)
}

func (fs *LocalFS) Search(_ context.Context, query string, opts types.SearchOpts) ([]types.SearchResult, error) {
	var results []types.SearchResult
	root := fs.hostPath("")
	lowerQuery := strings.ToLower(query)

	filepath.WalkDir(root, func(hp string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(strings.ToLower(d.Name()), lowerQuery) {
			relPath, _ := filepath.Rel(root, hp)
			relPath = filepath.ToSlash(relPath)
			info, infoErr := d.Info()
			if infoErr != nil {
				return nil
			}
			results = append(results, types.SearchResult{Entry: *fs.infoToEntry(relPath, info), Score: 1.0})
		}
		if opts.MaxResults > 0 && len(results) >= opts.MaxResults {
			return filepath.SkipAll
		}
		return nil
	})

	return results, nil
}

func (fs *LocalFS) infoToEntry(vosPath string, info os.FileInfo) *types.Entry {
	perm := fs.perm
	if info.IsDir() && perm.CanRead() {
		perm = perm | types.PermExec
	}
	return &types.Entry{
		Name: info.Name(), Path: vosPath, IsDir: info.IsDir(), Perm: perm,
		Size: info.Size(), Modified: info.ModTime(),
	}
}

func (fs *LocalFS) MountInfo() (string, string) { return "localfs", fs.root }
