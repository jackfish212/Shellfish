package mounts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackfish212/grasp/types"
)

var (
	_ types.Provider   = (*MemFS)(nil)
	_ types.Readable   = (*MemFS)(nil)
	_ types.Writable   = (*MemFS)(nil)
	_ types.Executable = (*MemFS)(nil)
	_ types.Mutable    = (*MemFS)(nil)
	_ types.Touchable  = (*MemFS)(nil)
)

// Func is the signature for functions registered as binaries.
type Func func(ctx context.Context, args []string, stdin string) (string, error)

// ExecFunc is the streaming variant of Func.
type ExecFunc func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)

// FuncMeta contains metadata about a registered function.
type FuncMeta struct {
	Description string
	Usage       string
}

// MemFS is an in-memory filesystem.
type MemFS struct {
	mu    sync.RWMutex
	files map[string]*memFile
	perm  types.Perm
}

type memFile struct {
	content  []byte
	isDir    bool
	perm     types.Perm
	modified time.Time
	meta     map[string]string
	fn       Func
	execFn   ExecFunc
}

// NewMemFS creates a new in-memory filesystem.
func NewMemFS(perm types.Perm) *MemFS {
	return &MemFS{files: make(map[string]*memFile), perm: perm}
}

func (fs *MemFS) AddFile(path string, content []byte, perm types.Perm) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.files[normPath(path)] = &memFile{content: content, perm: perm, modified: time.Now()}
	slog.Debug("memfs: added file", "path", path, "size", len(content), "perm", perm)
}

func (fs *MemFS) AddDir(path string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.files[normPath(path)] = &memFile{isDir: true, perm: types.PermRX, modified: time.Now()}
	slog.Debug("memfs: added directory", "path", path)
}

func (fs *MemFS) AddFunc(path string, fn Func, meta FuncMeta) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.files[normPath(path)] = &memFile{
		perm:     types.PermRX,
		modified: time.Now(),
		meta:     map[string]string{"kind": "func", "description": meta.Description},
		fn:       fn,
	}
	if meta.Usage != "" {
		fs.files[normPath(path)].meta["usage"] = meta.Usage
	}
}

func (fs *MemFS) AddExecFunc(path string, fn ExecFunc, meta FuncMeta) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.files[normPath(path)] = &memFile{
		perm:     types.PermRX,
		modified: time.Now(),
		meta:     map[string]string{"kind": "func", "description": meta.Description},
		execFn:   fn,
	}
	slog.Debug("memfs: added exec function", "path", path, "description", meta.Description, "usage", meta.Usage)
	if meta.Usage != "" {
		fs.files[normPath(path)].meta["usage"] = meta.Usage
	}
}

func (fs *MemFS) RemoveFunc(path string) bool {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if _, ok := fs.files[normPath(path)]; ok {
		delete(fs.files, normPath(path))
		return true
	}
	return false
}

func (fs *MemFS) Stat(_ context.Context, path string) (*types.Entry, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	path = normPath(path)

	if f, ok := fs.files[path]; ok {
		return f.toEntry(path), nil
	}

	prefix := path + "/"
	if path == "" {
		prefix = ""
	}
	for k := range fs.files {
		if strings.HasPrefix(k, prefix) {
			return &types.Entry{Name: baseName(path), Path: path, IsDir: true, Perm: types.PermRX}, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
}

func (fs *MemFS) List(_ context.Context, path string, _ types.ListOpts) ([]types.Entry, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	path = normPath(path)
	prefix := path + "/"
	if path == "" {
		prefix = ""
	}

	seen := make(map[string]bool)
	var entries []types.Entry

	for k, f := range fs.files {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := k[len(prefix):]
		if rest == "" {
			continue
		}

		name := rest
		isImplicitDir := false
		if idx := strings.IndexByte(rest, '/'); idx >= 0 {
			name = rest[:idx]
			isImplicitDir = true
		}

		if seen[name] {
			continue
		}
		seen[name] = true

		if isImplicitDir {
			entries = append(entries, types.Entry{Name: name, Path: prefix + name, IsDir: true, Perm: types.PermRX})
		} else {
			entries = append(entries, *f.toEntry(prefix + name))
		}
	}

	if path != "" && len(entries) == 0 {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func (fs *MemFS) Open(_ context.Context, path string) (types.File, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	p := normPath(path)
	f, ok := fs.files[p]
	if !ok {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}

	entry := f.toEntry(p)

	if f.fn != nil || f.execFn != nil {
		help := fs.formatHelp(p, f)
		base := types.NewFile(p, entry, io.NopCloser(strings.NewReader(help)))

		fn, execFn := f.fn, f.execFn
		return types.NewExecutableFile(base, func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
			if execFn != nil {
				return execFn(ctx, args, stdin)
			}
			var stdinStr string
			if stdin != nil {
				data, err := io.ReadAll(stdin)
				if err == nil {
					stdinStr = string(data)
				}
			}
			output, err := fn(ctx, args, stdinStr)
			if err != nil {
				return io.NopCloser(strings.NewReader(fmt.Sprintf("error: %v\n", err))), nil
			}
			return io.NopCloser(strings.NewReader(output)), nil
		}), nil
	}

	if !f.perm.CanRead() {
		return nil, fmt.Errorf("%w: %s", types.ErrNotReadable, path)
	}

	br := bytes.NewReader(f.content)
	rc := io.NopCloser(br)
	return types.NewSeekableFile(p, entry, rc, br), nil
}

func (fs *MemFS) Write(_ context.Context, path string, r io.Reader) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if existing, ok := fs.files[normPath(path)]; ok && (existing.fn != nil || existing.execFn != nil) {
		return fmt.Errorf("%w: %s (use RemoveFunc first)", types.ErrNotWritable, path)
	}

	p := normPath(path)
	if existing, ok := fs.files[p]; ok {
		existing.content = data
		existing.modified = time.Now()
	} else {
		fs.files[p] = &memFile{content: data, perm: fs.perm, modified: time.Now()}
	}
	return nil
}

func (fs *MemFS) Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error) {
	fs.mu.RLock()
	f, ok := fs.files[normPath(path)]
	fs.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", types.ErrNotExecutable, path)
	}

	if f.execFn != nil {
		return f.execFn(ctx, args, stdin)
	}

	if f.fn == nil {
		return nil, fmt.Errorf("%w: %s (not executable)", types.ErrNotExecutable, path)
	}

	var stdinStr string
	if stdin != nil {
		data, err := io.ReadAll(stdin)
		if err == nil {
			stdinStr = string(data)
		}
	}

	output, err := f.fn(ctx, args, stdinStr)
	if err != nil {
		return io.NopCloser(strings.NewReader(fmt.Sprintf("error: %v\n", err))), nil
	}
	return io.NopCloser(strings.NewReader(output)), nil
}

func (fs *MemFS) Mkdir(_ context.Context, path string, perm types.Perm) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	p := normPath(path)
	if p == "" {
		return fmt.Errorf("%w: cannot mkdir root", types.ErrNotSupported)
	}
	if _, ok := fs.files[p]; ok {
		return fmt.Errorf("%w: %s", types.ErrAlreadyMounted, p)
	}
	fs.files[p] = &memFile{isDir: true, perm: perm, modified: time.Now()}
	return nil
}

func (fs *MemFS) Remove(_ context.Context, path string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	p := normPath(path)
	if p == "" {
		return fmt.Errorf("%w: cannot remove root", types.ErrNotSupported)
	}

	_, exists := fs.files[p]
	prefix := p + "/"
	hasChildren := false
	for k := range fs.files {
		if strings.HasPrefix(k, prefix) {
			hasChildren = true
			break
		}
	}

	if !exists && !hasChildren {
		return fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}

	delete(fs.files, p)
	for k := range fs.files {
		if strings.HasPrefix(k, prefix) {
			delete(fs.files, k)
		}
	}
	return nil
}

func (fs *MemFS) Rename(_ context.Context, oldPath, newPath string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	old := normPath(oldPath)
	nw := normPath(newPath)
	if old == "" || nw == "" {
		return fmt.Errorf("%w: cannot rename root", types.ErrNotSupported)
	}

	f, exists := fs.files[old]
	if !exists {
		return fmt.Errorf("%w: %s", types.ErrNotFound, oldPath)
	}

	delete(fs.files, old)
	fs.files[nw] = f
	f.modified = time.Now()

	oldPrefix := old + "/"
	newPrefix := nw + "/"
	for k, v := range fs.files {
		if strings.HasPrefix(k, oldPrefix) {
			rest := k[len(oldPrefix):]
			delete(fs.files, k)
			fs.files[newPrefix+rest] = v
		}
	}
	return nil
}

func (fs *MemFS) Touch(_ context.Context, path string) error {
	if !fs.perm.CanWrite() {
		return fmt.Errorf("%w: %s", types.ErrNotWritable, path)
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	p := normPath(path)
	if p == "" {
		return fmt.Errorf("%w: cannot touch root", types.ErrNotSupported)
	}

	if f, ok := fs.files[p]; ok {
		f.modified = time.Now()
	} else {
		fs.files[p] = &memFile{content: []byte{}, perm: fs.perm, modified: time.Now()}
	}
	return nil
}

func (f *memFile) toEntry(path string) *types.Entry {
	return &types.Entry{
		Name: baseName(path), Path: path, IsDir: f.isDir, Perm: f.perm,
		Size: int64(len(f.content)), Modified: f.modified, Meta: f.meta,
	}
}

func (fs *MemFS) formatHelp(name string, f *memFile) string {
	var buf strings.Builder
	desc := f.meta["description"]
	usage := f.meta["usage"]
	fmt.Fprintf(&buf, "%s — %s\n", name, desc)
	if usage != "" {
		fmt.Fprintf(&buf, "\nUsage: %s\n", usage)
	}
	return buf.String()
}

func (fs *MemFS) MountInfo() (string, string) { return "memfs", "in-memory" }

// ErrFuncFailed is returned by a registered function to indicate failure.
type ErrFuncFailed string

func (e ErrFuncFailed) Error() string { return string(e) }
