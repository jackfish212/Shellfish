package shellfish

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	stdpath "path"
	"sort"
	"strings"

	"github.com/jackfish212/shellfish/shell"
)

// VirtualOS is the top-level orchestrator. It owns the mount table and
// provides unified operations that transparently handle virtual directories,
// mount merging, permission checking, and capability detection.
type VirtualOS struct {
	mounts *MountTable
	hub    *watchHub
}

// New creates a new VirtualOS instance.
func New() *VirtualOS {
	return &VirtualOS{mounts: NewMountTable(), hub: newWatchHub()}
}

// Watch creates a Watcher that receives events for paths under prefix
// matching the given event mask. Use "/" or "" to watch all paths.
func (v *VirtualOS) Watch(prefix string, mask EventType) *Watcher {
	return v.hub.watch(prefix, mask)
}

// Notify emits a filesystem watch event. Use this for providers that generate
// content autonomously (e.g., RSS polling, webhooks) and need to notify watchers.
func (v *VirtualOS) Notify(evType EventType, path string) {
	v.hub.emit(evType, CleanPath(path))
}

// Mount registers a Provider at the given path.
func (v *VirtualOS) Mount(path string, p Provider) error {
	path = CleanPath(path)

	if path == "/" {
		return v.mounts.Mount(path, p)
	}

	if _, inner, err := v.mounts.Resolve(path); err == nil && inner == "" {
		return fmt.Errorf("%w: %s is already a mount point", ErrAlreadyMounted, path)
	}

	parent := stdpath.Dir(path)
	parent = CleanPath(parent)

	// Check if parent path is resolvable or is a virtual directory (from other mounts)
	_, _, parentErr := v.mounts.Resolve(parent)
	if parentErr != nil {
		// Parent doesn't exist in any filesystem, check if it's a virtual parent
		if children := v.mounts.ChildMounts(parent); len(children) == 0 {
			// Special case: mounting to empty root
			if parent == "/" && len(v.mounts.All()) == 0 {
				return v.mounts.Mount(path, p)
			}
			return fmt.Errorf("%w: %s", ErrParentNotExist, parent)
		}
	}

	// Mount points are virtual directories and don't need to exist
	// in the parent filesystem. The mount table will create them as
	// virtual entries automatically via ChildMounts().
	return v.mounts.Mount(path, p)
}

// Unmount removes the mount at the given path.
func (v *VirtualOS) Unmount(path string) error {
	return v.mounts.Unmount(path)
}

// MountTable returns the underlying mount table for inspection.
func (v *VirtualOS) MountTable() *MountTable {
	return v.mounts
}

// Stat returns entry metadata.
func (v *VirtualOS) Stat(ctx context.Context, path string) (*Entry, error) {
	path = CleanPath(path)

	if p, inner, err := v.mounts.Resolve(path); err == nil {
		// If inner is empty, this is a mount point itself - always return as directory
		if inner == "" {
			return &Entry{
				Name:  baseName(path),
				Path:  path,
				IsDir: true,
				Perm:  PermRW, // Mount points are always readable/writable
			}, nil
		}
		if entry, statErr := p.Stat(ctx, inner); statErr == nil {
			entry.Path = path
			return entry, nil
		}
	}

	if children := v.mounts.ChildMounts(path); len(children) > 0 {
		return &Entry{
			Name:  baseName(path),
			Path:  path,
			IsDir: true,
			Perm:  PermRX,
		}, nil
	}

	return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
}

// List returns entries at a path, merging provider entries with virtual directories.
func (v *VirtualOS) List(ctx context.Context, path string, opts ListOpts) ([]Entry, error) {
	path = CleanPath(path)

	var entries []Entry
	seen := make(map[string]bool)
	resolved := false

	if p, inner, err := v.mounts.Resolve(path); err == nil {
		resolved = true
		if provEntries, listErr := p.List(ctx, inner, opts); listErr == nil {
			for _, e := range provEntries {
				if !strings.HasPrefix(e.Path, "/") {
					e.Path = CleanPath(path + "/" + e.Name)
				}
				entries = append(entries, e)
				seen[e.Name] = true
			}
		}
	}

	for _, child := range v.mounts.ChildMounts(path) {
		if !seen[child.Name] {
			entries = append(entries, child)
			seen[child.Name] = true
		}
	}

	if !resolved && len(entries) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	return entries, nil
}

// OpenFile opens a file with the given flags.
func (v *VirtualOS) OpenFile(ctx context.Context, path string, flag OpenFlag) (File, error) {
	path = CleanPath(path)

	p, inner, err := v.mounts.Resolve(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	if flag.IsReadable() && !flag.IsWritable() {
		r, ok := p.(Readable)
		if !ok {
			return nil, fmt.Errorf("%w: %s (provider is not readable)", ErrNotReadable, path)
		}
		if entry, statErr := p.Stat(ctx, inner); statErr == nil {
			if !entry.Perm.CanRead() {
				return nil, fmt.Errorf("%w: %s", ErrNotReadable, path)
			}
		}
		return r.Open(ctx, inner)
	}

	if flag.IsWritable() {
		w, ok := p.(Writable)
		if !ok {
			return nil, fmt.Errorf("%w: %s (provider is not writable)", ErrNotWritable, path)
		}
		if entry, statErr := p.Stat(ctx, inner); statErr == nil {
			if !entry.Perm.CanWrite() {
				return nil, fmt.Errorf("%w: %s", ErrNotWritable, path)
			}
		} else if !flag.Has(O_CREATE) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return newWritableFile(path, inner, w, flag), nil
	}

	return nil, fmt.Errorf("%w: invalid open flags for %s", ErrNotSupported, path)
}

// Open opens a file for reading.
func (v *VirtualOS) Open(ctx context.Context, path string) (File, error) {
	path = CleanPath(path)

	p, inner, err := v.mounts.Resolve(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	r, ok := p.(Readable)
	if !ok {
		return nil, fmt.Errorf("%w: %s (provider is not readable)", ErrNotReadable, path)
	}

	if entry, statErr := p.Stat(ctx, inner); statErr == nil {
		if !entry.Perm.CanRead() {
			return nil, fmt.Errorf("%w: %s", ErrNotReadable, path)
		}
	}

	return r.Open(ctx, inner)
}

// Write writes content to a path.
func (v *VirtualOS) Write(ctx context.Context, path string, reader io.Reader) error {
	path = CleanPath(path)

	p, inner, err := v.mounts.Resolve(path)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	w, ok := p.(Writable)
	if !ok {
		return fmt.Errorf("%w: %s (provider is not writable)", ErrNotWritable, path)
	}

	existing, statErr := p.Stat(ctx, inner)
	isNew := statErr != nil
	if existing != nil && !existing.Perm.CanWrite() {
		return fmt.Errorf("%w: %s", ErrNotWritable, path)
	}

	if err := w.Write(ctx, inner, reader); err != nil {
		return err
	}
	if isNew {
		v.hub.emit(EventCreate, path)
	}
	v.hub.emit(EventWrite, path)
	return nil
}

// Exec executes an entry at the given path.
func (v *VirtualOS) Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error) {
	path = CleanPath(path)

	p, inner, err := v.mounts.Resolve(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	x, ok := p.(Executable)
	if !ok {
		return nil, fmt.Errorf("%w: %s (provider is not executable)", ErrNotExecutable, path)
	}

	entry, statErr := p.Stat(ctx, inner)
	if statErr != nil {
		return nil, statErr
	}
	if !entry.Perm.CanExec() {
		return nil, fmt.Errorf("%w: %s (%s)", ErrNotExecutable, path, entry.Perm)
	}

	return x.Exec(ctx, inner, args, stdin)
}

// Mkdir creates a directory at the given path.
func (v *VirtualOS) Mkdir(ctx context.Context, path string, perm Perm) error {
	path = CleanPath(path)

	p, inner, err := v.mounts.Resolve(path)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	m, ok := p.(Mutable)
	if !ok {
		return fmt.Errorf("%w: %s (provider is not mutable)", ErrNotSupported, path)
	}

	if err := m.Mkdir(ctx, inner, perm); err != nil {
		return err
	}
	v.hub.emit(EventMkdir, path)
	return nil
}

// Remove removes a file or directory at the given path.
func (v *VirtualOS) Remove(ctx context.Context, path string) error {
	path = CleanPath(path)

	p, inner, err := v.mounts.Resolve(path)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	m, ok := p.(Mutable)
	if !ok {
		return fmt.Errorf("%w: %s (provider is not mutable)", ErrNotSupported, path)
	}

	if entry, statErr := p.Stat(ctx, inner); statErr == nil {
		if !entry.Perm.CanWrite() {
			return fmt.Errorf("%w: %s", ErrNotWritable, path)
		}
	}

	if err := m.Remove(ctx, inner); err != nil {
		return err
	}
	v.hub.emit(EventRemove, path)
	return nil
}

// Rename moves/renames an entry.
func (v *VirtualOS) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPath = CleanPath(oldPath)
	newPath = CleanPath(newPath)

	pOld, innerOld, err := v.mounts.Resolve(oldPath)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotFound, oldPath)
	}

	pNew, innerNew, err := v.mounts.Resolve(newPath)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotFound, newPath)
	}

	if pOld != pNew {
		return fmt.Errorf("%w: cross-mount rename not supported (%s → %s)", ErrNotSupported, oldPath, newPath)
	}

	m, ok := pOld.(Mutable)
	if !ok {
		return fmt.Errorf("%w: %s (provider is not mutable)", ErrNotSupported, oldPath)
	}

	if err := m.Rename(ctx, innerOld, innerNew); err != nil {
		return err
	}
	v.hub.emitRename(EventRename, newPath, oldPath)
	return nil
}

// Touch updates the modification time of a file, or creates it if it doesn't exist.
// If the provider implements Touchable, it uses the efficient native implementation.
// Otherwise, it falls back to reading and rewriting the file content (or creating empty).
func (v *VirtualOS) Touch(ctx context.Context, path string) error {
	path = CleanPath(path)

	p, inner, err := v.mounts.Resolve(path)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	_, statErr := p.Stat(ctx, inner)
	isNew := statErr != nil

	// Fast path: provider implements Touchable
	if t, ok := p.(Touchable); ok {
		if err := t.Touch(ctx, inner); err != nil {
			return err
		}
		if isNew {
			v.hub.emit(EventCreate, path)
		}
		v.hub.emit(EventWrite, path)
		return nil
	}

	// Fallback: use Write to update timestamp or create empty file
	w, wOk := p.(Writable)
	if !wOk {
		return fmt.Errorf("%w: %s (provider supports neither Touch nor Write)", ErrNotSupported, path)
	}

	// If file exists and is readable, read content and rewrite to update timestamp
	if r, rOk := p.(Readable); rOk {
		if !isNew {
			f, openErr := r.Open(ctx, inner)
			if openErr == nil {
				data, _ := io.ReadAll(f)
				f.Close()
				if err := w.Write(ctx, inner, bytes.NewReader(data)); err != nil {
					return err
				}
				v.hub.emit(EventWrite, path)
				return nil
			}
		}
	}

	// File doesn't exist or not readable, create empty file
	if err := w.Write(ctx, inner, strings.NewReader("")); err != nil {
		return err
	}
	v.hub.emit(EventCreate, path)
	v.hub.emit(EventWrite, path)
	return nil
}

// Search performs a cross-mount search.
func (v *VirtualOS) Search(ctx context.Context, query string, opts SearchOpts) ([]SearchResult, error) {
	mountPaths := v.mounts.All()

	type result struct {
		results []SearchResult
		err     error
	}

	ch := make(chan result, len(mountPaths))

	for _, mp := range mountPaths {
		go func(mountPath string) {
			if opts.Scope != "" && !strings.HasPrefix(mountPath, CleanPath(opts.Scope)) {
				ch <- result{}
				return
			}

			p, _, resolveErr := v.mounts.Resolve(mountPath)
			if resolveErr != nil {
				ch <- result{err: resolveErr}
				return
			}

			s, ok := p.(Searchable)
			if !ok {
				ch <- result{}
				return
			}

			rs, searchErr := s.Search(ctx, query, opts)
			for i := range rs {
				if !strings.HasPrefix(rs[i].Entry.Path, "/") {
					if rs[i].Entry.Path == "" {
						rs[i].Entry.Path = mountPath
					} else {
						rs[i].Entry.Path = mountPath + "/" + rs[i].Entry.Path
					}
				}
			}
			ch <- result{results: rs, err: searchErr}
		}(mp)
	}

	var all []SearchResult
	var errs []error
	for range mountPaths {
		r := <-ch
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		all = append(all, r.results...)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Score > all[j].Score
	})

	if opts.MaxResults > 0 && len(all) > opts.MaxResults {
		all = all[:opts.MaxResults]
	}

	return all, errors.Join(errs...)
}

// Shell creates a new Shell bound to this VOS.
func (v *VirtualOS) Shell(user string) *shell.Shell {
	return shell.NewShell(v, user)
}
