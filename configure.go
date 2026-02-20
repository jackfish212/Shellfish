package shellfish

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/jackfish212/shellfish/mounts"
	"github.com/jackfish212/shellfish/types"
)

const defaultPath = "/usr/bin:/sbin"

// MountRootFS mounts a root filesystem with standard directory structure
// at "/" on the given VirtualOS. Returns the MemFS provider for further customization.
func MountRootFS(v *VirtualOS) (*mounts.MemFS, error) {
	memRoot := mounts.NewMemFS(PermRW)

	if err := v.Mount("/", memRoot); err != nil {
		return nil, err
	}

	dirs := []string{
		"bin",
		"sbin",
		"usr",
		"usr/bin",
		"etc",
		"home",
		"root",
		"tmp",
		"run",
		"var",
		"var/log",
		"var/tmp",
		"dev",
		"sys",
		"proc",
	}

	for _, dir := range dirs {
		memRoot.AddDir(dir)
	}

	profileContent := "export PATH=" + defaultPath + "\n"
	memRoot.AddFile("/etc/profile", []byte(profileContent), PermRO)

	return memRoot, nil
}

// Configure sets up a VirtualOS with standard filesystem structure,
// /proc, and all built-in commands. Returns the root MemFS for further customization.
func Configure(v *VirtualOS) (*mounts.MemFS, error) {
	slog.Debug("shellfish: starting configuration")
	rootFS, err := MountRootFS(v)
	if err != nil {
		slog.Error("shellfish: failed to mount root filesystem", "error", err)
		return nil, err
	}
	slog.Info("shellfish: root filesystem mounted")

	if err := MountProc(v); err != nil {
		slog.Error("shellfish: failed to mount /proc", "error", err)
		return nil, err
	}
	slog.Info("shellfish: /proc mounted")

	slog.Debug("shellfish: configuration complete")
	return rootFS, nil
}

// ─── /proc filesystem ───

type ProcProvider struct {
	mu    sync.RWMutex
	files map[string]*procFile
}

type procFile struct {
	content func() string
	perm    Perm
	entry   *Entry
}

func NewProcProvider() *ProcProvider {
	p := &ProcProvider{
		files: make(map[string]*procFile),
	}

	p.register("version", func() string {
		return GetVersionInfo().ProcVersion()
	}, PermRO)

	return p
}

func (p *ProcProvider) register(name string, content func() string, perm Perm) {
	p.files[name] = &procFile{
		content: content,
		perm:    perm,
		entry: &Entry{
			Name: name,
			Perm: perm,
		},
	}
}

func (p *ProcProvider) Stat(ctx context.Context, path string) (*Entry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if path == "" || path == "/" || path == "." {
		return &Entry{
			Name:  "proc",
			IsDir: true,
			Perm:  PermRO,
		}, nil
	}
	path = trimSlash(path)
	if f, ok := p.files[path]; ok {
		return f.entry, nil
	}
	return nil, fmt.Errorf("proc: %s: no such file", path)
}

func (p *ProcProvider) List(ctx context.Context, path string, _ ListOpts) ([]Entry, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if path != "" && path != "/" && path != "." {
		return nil, fmt.Errorf("proc: %s: not a directory", path)
	}

	entries := make([]Entry, 0, len(p.files))
	for name, f := range p.files {
		entries = append(entries, Entry{
			Name: name,
			Perm: f.perm,
		})
	}
	return entries, nil
}

func (p *ProcProvider) Open(ctx context.Context, path string) (File, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	path = trimSlash(path)
	if f, ok := p.files[path]; ok {
		content := f.content()
		return types.NewFile(path, f.entry, io.NopCloser(bytes.NewReader([]byte(content)))), nil
	}
	return nil, fmt.Errorf("proc: %s: no such file", path)
}

func MountProc(v *VirtualOS) error {
	return v.Mount("/proc", NewProcProvider())
}

func trimSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

// ─── Version info ───

var (
	version   = "dev"
	buildDate = ""
	gitCommit = ""
)

type VersionInfo struct {
	Version   string
	BuildDate string
	GitCommit string
	GoVersion string
	Platform  string
}

func GetVersionInfo() VersionInfo {
	bd := buildDate
	if bd == "" {
		bd = time.Now().Format("2006-01-02")
	}
	return VersionInfo{
		Version:   version,
		BuildDate: bd,
		GitCommit: gitCommit,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

func (v VersionInfo) ProcVersion() string {
	commit := v.GitCommit
	if commit != "" && len(commit) > 8 {
		commit = commit[:8]
	}
	if commit != "" {
		commit = " (" + commit + ")"
	}
	return fmt.Sprintf("AgentFS version %s%s (Go %s) #1 %s",
		v.Version, commit, v.GoVersion, v.BuildDate)
}
