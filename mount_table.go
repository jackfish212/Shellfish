package grasp

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
)

type mountRecord struct {
	path     string
	provider Provider
}

// MountInfo holds detailed information about a mount point.
type MountInfo struct {
	Path        string
	Provider    Provider
	Permissions string
}

// MountTable manages all mount points and resolves arbitrary paths to the
// correct Provider plus the remaining inner path.
type MountTable struct {
	mu      sync.RWMutex
	records []mountRecord
	rcache  resolveCache
}

type resolveCache struct {
	mu    sync.RWMutex
	items map[string]resolveEntry
}

type resolveEntry struct {
	provider Provider
	inner    string
}

func (c *resolveCache) get(path string) (Provider, string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.items == nil {
		return nil, "", false
	}
	e, ok := c.items[path]
	if !ok {
		return nil, "", false
	}
	return e.provider, e.inner, true
}

func (c *resolveCache) put(path string, p Provider, inner string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.items == nil {
		c.items = make(map[string]resolveEntry)
	}
	c.items[path] = resolveEntry{provider: p, inner: inner}
}

func (c *resolveCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = nil
}

// NewMountTable creates an empty mount table.
func NewMountTable() *MountTable {
	return &MountTable{}
}

// Mount registers a Provider at the given path.
func (t *MountTable) Mount(mountPath string, p Provider) error {
	mountPath = CleanPath(mountPath)

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, r := range t.records {
		if r.path == mountPath {
			return fmt.Errorf("%w: %s", ErrAlreadyMounted, mountPath)
		}
	}

	for _, r := range t.records {
		if strings.HasPrefix(r.path, mountPath+"/") {
			return fmt.Errorf("%w: %s is ancestor of existing mount %s", ErrMountUnderMount, mountPath, r.path)
		}
	}

	t.records = append(t.records, mountRecord{path: mountPath, provider: p})

	sort.Slice(t.records, func(i, j int) bool {
		return len(t.records[i].path) > len(t.records[j].path)
	})

	t.rcache.invalidate()
	return nil
}

// Unmount removes the mount at the given path.
func (t *MountTable) Unmount(mountPath string) error {
	mountPath = CleanPath(mountPath)

	t.mu.Lock()
	defer t.mu.Unlock()

	for i, r := range t.records {
		if r.path == mountPath {
			t.records = append(t.records[:i], t.records[i+1:]...)
			t.rcache.invalidate()
			return nil
		}
	}
	return fmt.Errorf("%w: mount %s", ErrNotFound, mountPath)
}

// Resolve finds the provider and inner path for a given full path.
func (t *MountTable) Resolve(fullPath string) (Provider, string, error) {
	fullPath = CleanPath(fullPath)

	if p, inner, ok := t.rcache.get(fullPath); ok {
		return p, inner, nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, r := range t.records {
		if fullPath == r.path {
			t.rcache.put(fullPath, r.provider, "")
			return r.provider, "", nil
		}
		if r.path == "/" {
			inner := fullPath[1:]
			t.rcache.put(fullPath, r.provider, inner)
			return r.provider, inner, nil
		}
		if strings.HasPrefix(fullPath, r.path+"/") {
			inner := fullPath[len(r.path)+1:]
			t.rcache.put(fullPath, r.provider, inner)
			return r.provider, inner, nil
		}
	}
	return nil, "", fmt.Errorf("%w: no mount for %s", ErrNotFound, fullPath)
}

// ChildMounts returns virtual directory entries for mount points directly
// under dirPath.
func (t *MountTable) ChildMounts(dirPath string) []Entry {
	dirPath = CleanPath(dirPath)

	prefix := dirPath + "/"
	if dirPath == "/" {
		prefix = "/"
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	seen := make(map[string]bool)
	var entries []Entry

	for _, r := range t.records {
		var rest string
		if dirPath == "/" {
			rest = strings.TrimPrefix(r.path, "/")
		} else if strings.HasPrefix(r.path, prefix) {
			rest = r.path[len(prefix):]
		} else {
			continue
		}

		if rest == "" {
			continue
		}

		name := rest
		if idx := strings.IndexByte(rest, '/'); idx >= 0 {
			name = rest[:idx]
		}

		if seen[name] {
			continue
		}
		seen[name] = true

		entryPath := path.Join(dirPath, name)
		entries = append(entries, Entry{
			Name:  name,
			Path:  entryPath,
			IsDir: true,
			Perm:  PermRX,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}

// All returns every registered mount path.
func (t *MountTable) All() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	paths := make([]string, len(t.records))
	for i, r := range t.records {
		paths[i] = r.path
	}
	return paths
}

// AllInfo returns detailed information about all mount points.
func (t *MountTable) AllInfo() []MountInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	infos := make([]MountInfo, len(t.records))
	for i, r := range t.records {
		infos[i] = MountInfo{
			Path:     r.path,
			Provider: r.provider,
		}
		switch {
		case implementsWritable(r.provider) && implementsExecutable(r.provider):
			infos[i].Permissions = "rwx"
		case implementsReadable(r.provider) && implementsWritable(r.provider):
			infos[i].Permissions = "rw-"
		case implementsReadable(r.provider) && implementsExecutable(r.provider):
			infos[i].Permissions = "r-x"
		case implementsReadable(r.provider):
			infos[i].Permissions = "r--"
		default:
			infos[i].Permissions = "---"
		}
	}
	return infos
}

func implementsReadable(p Provider) bool {
	_, ok := p.(Readable)
	return ok
}

func implementsWritable(p Provider) bool {
	_, ok := p.(Writable)
	return ok
}

func implementsExecutable(p Provider) bool {
	_, ok := p.(Executable)
	return ok
}
