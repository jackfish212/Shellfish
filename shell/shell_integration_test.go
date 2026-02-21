package shell

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jackfish212/grasp/types"
)

// mockVirtualOS implements VirtualOS for testing
type mockVirtualOS struct {
	files    map[string]*mockFile
	dirs     map[string]bool
	execFile map[string]struct {
		fn    func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
		perms types.Perm
	}
}

type mockFile struct {
	content []byte
	perm    types.Perm
}

func newMockVirtualOS() *mockVirtualOS {
	return &mockVirtualOS{
		files: make(map[string]*mockFile),
		dirs:  make(map[string]bool),
		execFile: make(map[string]struct {
			fn    func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
			perms types.Perm
		}),
	}
}

func (m *mockVirtualOS) Stat(ctx context.Context, path string) (*types.Entry, error) {
	path = cleanPath(path)
	if path == "/" || m.dirs[path] {
		return &types.Entry{Name: path, Path: path, IsDir: true, Perm: types.PermRWX}, nil
	}
	if f, ok := m.files[path]; ok {
		return &types.Entry{Name: path, Path: path, IsDir: false, Size: int64(len(f.content)), Perm: f.perm}, nil
	}
	if _, ok := m.execFile[path]; ok {
		return &types.Entry{Name: path, Path: path, IsDir: false, Perm: types.PermRWX}, nil
	}
	return nil, types.ErrNotFound
}

func (m *mockVirtualOS) List(ctx context.Context, path string, opts types.ListOpts) ([]types.Entry, error) {
	path = cleanPath(path)
	if path != "/" && !m.dirs[path] {
		return nil, types.ErrNotDir
	}
	var entries []types.Entry
	seen := make(map[string]bool)
	for p := range m.files {
		if strings.HasPrefix(p, path) && p != path {
			rel := strings.TrimPrefix(p, path)
			if path != "/" {
				rel = strings.TrimPrefix(rel, "/")
			}
			parts := strings.Split(rel, "/")
			name := parts[0]
			if !seen[name] {
				isDir := len(parts) > 1
				entries = append(entries, types.Entry{Name: name, Path: path + "/" + name, IsDir: isDir, Perm: types.PermRW})
				seen[name] = true
			}
		}
	}
	for p := range m.dirs {
		if strings.HasPrefix(p, path) && p != path {
			rel := strings.TrimPrefix(p, path)
			if path != "/" {
				rel = strings.TrimPrefix(rel, "/")
			}
			parts := strings.Split(rel, "/")
			name := parts[0]
			if !seen[name] {
				entries = append(entries, types.Entry{Name: name, Path: path + "/" + name, IsDir: true, Perm: types.PermRWX})
				seen[name] = true
			}
		}
	}
	for p := range m.execFile {
		if strings.HasPrefix(p, path) && p != path {
			rel := strings.TrimPrefix(p, path)
			if path != "/" {
				rel = strings.TrimPrefix(rel, "/")
			}
			parts := strings.Split(rel, "/")
			name := parts[0]
			if len(parts) == 1 && !seen[name] {
				entries = append(entries, types.Entry{Name: name, Path: path + "/" + name, IsDir: false, Perm: types.PermRWX})
				seen[name] = true
			}
		}
	}
	return entries, nil
}

func (m *mockVirtualOS) Open(ctx context.Context, path string) (types.File, error) {
	path = cleanPath(path)
	if f, ok := m.files[path]; ok {
		return types.NewFile(path, &types.Entry{Name: path, Path: path, Perm: f.perm}, io.NopCloser(bytes.NewReader(f.content))), nil
	}
	return nil, types.ErrNotFound
}

func (m *mockVirtualOS) Write(ctx context.Context, path string, reader io.Reader) error {
	path = cleanPath(path)
	data, _ := io.ReadAll(reader)
	m.files[path] = &mockFile{content: data, perm: types.PermRW}
	// Ensure parent dirs exist
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts)-1; i++ {
		parent := strings.Join(parts[:i+1], "/")
		m.dirs[parent] = true
	}
	return nil
}

func (m *mockVirtualOS) Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error) {
	path = cleanPath(path)
	if e, ok := m.execFile[path]; ok {
		return e.fn(ctx, args, stdin)
	}
	return nil, types.ErrNotExecutable
}

func setupTestShell(t *testing.T) (*Shell, *mockVirtualOS) {
	t.Helper()
	v := newMockVirtualOS()
	v.dirs["/"] = true
	v.dirs["/bin"] = true
	v.dirs["/home"] = true
	v.dirs["/home/tester"] = true
	v.dirs["/tmp"] = true
	v.dirs["/etc"] = true

	// Add some builtins as exec files
	v.execFile["/bin/pwd"] = struct {
		fn    func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
		perms types.Perm
	}{
		fn: func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("/home/tester\n")), nil
		},
		perms: types.PermRWX,
	}

	v.execFile["/bin/cat"] = struct {
		fn    func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
		perms types.Perm
	}{
		fn: func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
			if len(args) > 0 {
				// Read file
				if f, ok := v.files[cleanPath(args[0])]; ok {
					return io.NopCloser(bytes.NewReader(f.content)), nil
				}
				return io.NopCloser(strings.NewReader("cat: " + args[0] + ": No such file\n")), nil
			}
			if stdin != nil {
				data, _ := io.ReadAll(stdin)
				return io.NopCloser(bytes.NewReader(data)), nil
			}
			return io.NopCloser(strings.NewReader("")), nil
		},
		perms: types.PermRWX,
	}

	v.execFile["/bin/echo"] = struct {
		fn    func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
		perms types.Perm
	}{
		fn: func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(strings.Join(args, " ") + "\n")), nil
		},
		perms: types.PermRWX,
	}

	v.execFile["/bin/head"] = struct {
		fn    func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
		perms types.Perm
	}{
		fn: func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
			if stdin != nil {
				data, _ := io.ReadAll(stdin)
				lines := strings.Split(string(data), "\n")
				if len(lines) > 2 {
					lines = lines[:2]
				}
				return io.NopCloser(strings.NewReader(strings.Join(lines, "\n") + "\n")), nil
			}
			return io.NopCloser(strings.NewReader("")), nil
		},
		perms: types.PermRWX,
	}

	v.files["/home/tester/hello.txt"] = &mockFile{content: []byte("hello world"), perm: types.PermRW}

	sh := NewShell(v, "tester")
	return sh, v
}

// ─── Shell Integration Tests ───

func TestShellIntegrationNewShell(t *testing.T) {
	sh, _ := setupTestShell(t)
	if sh == nil {
		t.Fatal("NewShell returned nil")
	}
	if sh.Cwd() != "/home/tester" {
		t.Errorf("Cwd = %q, want /home/tester", sh.Cwd())
	}
	if sh.Env.Get("USER") != "tester" {
		t.Errorf("USER = %q, want tester", sh.Env.Get("USER"))
	}
}

func TestShellIntegrationExecutePwd(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "pwd")
	if result.Code != 0 {
		t.Errorf("pwd should succeed, got code %d", result.Code)
	}
	if !strings.Contains(result.Output, "/home/tester") {
		t.Errorf("pwd output = %q", result.Output)
	}
}

func TestShellIntegrationExecuteEcho(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo hello world")
	if result.Code != 0 {
		t.Errorf("echo should succeed, got code %d", result.Code)
	}
	if strings.TrimSpace(result.Output) != "hello world" {
		t.Errorf("echo output = %q, want 'hello world'", result.Output)
	}
}

func TestShellIntegrationExecuteCat(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "cat /home/tester/hello.txt")
	if result.Code != 0 {
		t.Errorf("cat should succeed, got code %d", result.Code)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Errorf("cat output = %q", result.Output)
	}
}

func TestShellIntegrationRedirectWrite(t *testing.T) {
	sh, v := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo hello > /tmp/output.txt")
	if result.Code != 0 {
		t.Errorf("redirect should succeed, got code %d: %s", result.Code, result.Output)
	}

	f, err := v.files["/tmp/output.txt"]
	if !err || f == nil {
		t.Fatal("file should exist")
	}
	if string(f.content) != "hello\n" {
		t.Errorf("file content = %q, want 'hello\\n'", string(f.content))
	}
}

func TestShellIntegrationPipe(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo line1 line2 line3 | head")
	if result.Code != 0 {
		t.Errorf("pipe should succeed, got code %d", result.Code)
	}
}

func TestShellIntegrationLogicalAnd(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo first && echo second")
	if !strings.Contains(result.Output, "first") || !strings.Contains(result.Output, "second") {
		t.Errorf("&& should run both: %q", result.Output)
	}
}

func TestShellIntegrationLogicalOr(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo success || echo fallback")
	if strings.Contains(result.Output, "fallback") {
		t.Error("|| should not run fallback when first succeeds")
	}
}

func TestShellIntegrationEnvExpansion(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo $USER")
	if strings.TrimSpace(result.Output) != "tester" {
		t.Errorf("$USER expansion = %q, want tester", result.Output)
	}
}

func TestShellIntegrationTildeExpansion(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "cat ~/hello.txt")
	if !strings.Contains(result.Output, "hello world") {
		t.Errorf("tilde expansion failed: %q", result.Output)
	}
}

func TestShellIntegrationCommandNotFound(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "nonexistent_command")
	if result.Code == 0 {
		t.Error("nonexistent command should fail")
	}
}

func TestShellIntegrationHistory(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	sh.Execute(ctx, "echo first")
	sh.Execute(ctx, "echo second")

	if sh.HistorySize() < 2 {
		t.Errorf("HistorySize = %d, want at least 2", sh.HistorySize())
	}

	hist := sh.History()
	if len(hist) < 2 {
		t.Errorf("History length = %d, want at least 2", len(hist))
	}

	sh.ClearHistory()
	if sh.HistorySize() != 0 {
		t.Errorf("ClearHistory should clear, got size %d", sh.HistorySize())
	}
}

func TestShellIntegrationEmptyCommand(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "")
	if result.Code != 0 || result.Output != "" {
		t.Errorf("empty command should be no-op, got code=%d output=%q", result.Code, result.Output)
	}
}

func TestShellIntegrationCommandGroup(t *testing.T) {
	sh, _ := setupTestShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "{ echo a; echo b }")
	if !strings.Contains(result.Output, "a") || !strings.Contains(result.Output, "b") {
		t.Errorf("command group output = %q", result.Output)
	}
}

func TestShellIntegrationHereDoc(t *testing.T) {
	sh, v := setupTestShell(t)
	ctx := context.Background()

	heredoc := "cat > /tmp/heredoc.txt << EOF\nline1\nline2\nEOF"
	result := sh.Execute(ctx, heredoc)
	if result.Code != 0 {
		t.Errorf("heredoc should succeed, got code %d: %s", result.Code, result.Output)
	}

	f, ok := v.files["/tmp/heredoc.txt"]
	if !ok {
		t.Fatal("heredoc file should exist")
	}
	if !strings.Contains(string(f.content), "line1") || !strings.Contains(string(f.content), "line2") {
		t.Errorf("heredoc content = %q", string(f.content))
	}
}

func TestShellIntegrationGlobStar(t *testing.T) {
	sh, v := setupTestShell(t)
	ctx := context.Background()

	v.files["/home/tester/a.txt"] = &mockFile{content: []byte("aaa"), perm: types.PermRW}
	v.files["/home/tester/b.txt"] = &mockFile{content: []byte("bbb"), perm: types.PermRW}

	result := sh.Execute(ctx, "cat *.txt")
	if result.Code != 0 {
		t.Errorf("glob should succeed, got code %d: %s", result.Code, result.Output)
	}
}

func TestShellIntegrationWithEnv(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"TEST": "value"}
	ctx = WithEnv(ctx, env)

	val := Env(ctx, "TEST")
	if val != "value" {
		t.Errorf("Env(ctx, TEST) = %q, want 'value'", val)
	}

	val = Env(ctx, "NONEXISTENT")
	if val != "" {
		t.Errorf("Env(ctx, NONEXISTENT) = %q, want ''", val)
	}
}

// ─── Unit Tests for Path Functions ───

func TestCleanPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "/"},
		{".", "/"},
		{"/", "/"},
		{"/foo", "/foo"},
		{"/foo/", "/foo"},
		{"/foo/bar", "/foo/bar"},
		{"foo", "/foo"},
		{"foo/bar", "/foo/bar"},
		{"/foo/../bar", "/bar"},
		{`\foo\bar`, "/foo/bar"}, // Windows path conversion
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cleanPath(tt.input)
			if result != tt.expected {
				t.Errorf("cleanPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAbsPath(t *testing.T) {
	sh := &Shell{Env: NewShellEnv()}
	sh.Env.Set("PWD", "/home/tester")

	tests := []struct {
		input    string
		expected string
	}{
		{"/abs/path", "/abs/path"},
		{"relative", "/home/tester/relative"},
		{"./relative", "/home/tester/relative"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sh.absPath(tt.input)
			if result != tt.expected {
				t.Errorf("absPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ─── Unit Tests for HereDoc Functions ───

func TestParseHereDoc(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantDelim string
		wantCmd   string
	}{
		{
			name:      "simple heredoc",
			input:     "cat << EOF\nline1\nEOF",
			wantNil:   false,
			wantDelim: "EOF",
			wantCmd:   "cat",
		},
		{
			name:      "heredoc with redirect",
			input:     "cat > file.txt << EOF\nline1\nEOF",
			wantNil:   false,
			wantDelim: "EOF",
			wantCmd:   "cat > file.txt",
		},
		{
			name:    "no heredoc",
			input:   "echo hello",
			wantNil: true,
		},
		{
			name:      "quoted delimiter",
			input:     "cat << 'EOF'\nline1\nEOF",
			wantNil:   false,
			wantDelim: "EOF",
			wantCmd:   "cat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, cmd, _ := parseHereDoc(tt.input)
			if tt.wantNil {
				if info != nil {
					t.Error("expected nil heredoc info")
				}
				return
			}
			if info == nil {
				t.Fatal("expected non-nil heredoc info")
			}
			if info.delimiter != tt.wantDelim {
				t.Errorf("delimiter = %q, want %q", info.delimiter, tt.wantDelim)
			}
			if cmd != tt.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tt.wantCmd)
			}
		})
	}
}

func TestExtractHereDocContent(t *testing.T) {
	tests := []struct {
		name          string
		fullLine      string
		delim         string
		wantContent   string
		wantRemaining string
		wantErr       bool
	}{
		{
			name:        "simple",
			fullLine:    "cat << EOF\nline1\nline2\nEOF",
			delim:       "EOF",
			wantContent: "line1\nline2",
		},
		{
			name:          "with remaining",
			fullLine:      "cat << EOF\ncontent\nEOF\necho next",
			delim:         "EOF",
			wantContent:   "content",
			wantRemaining: "echo next",
		},
		{
			name:     "delimiter not found",
			fullLine: "cat << EOF\nline1\nline2",
			delim:    "EOF",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, remaining, err := extractHereDocContent(tt.fullLine, tt.delim)
			if tt.wantErr {
				if err == "" {
					t.Error("expected error, got none")
				}
				return
			}
			if err != "" {
				t.Fatalf("unexpected error: %s", err)
			}
			if content != tt.wantContent {
				t.Errorf("content = %q, want %q", content, tt.wantContent)
			}
			if remaining != tt.wantRemaining {
				t.Errorf("remaining = %q, want %q", remaining, tt.wantRemaining)
			}
		})
	}
}

// ─── ResolveCommand Tests ───

func TestResolveCommand(t *testing.T) {
	v := newMockVirtualOS()
	v.dirs["/"] = true
	v.dirs["/bin"] = true
	v.execFile["/bin/mycmd"] = struct {
		fn    func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error)
		perms types.Perm
	}{
		fn: func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("ok\n")), nil
		},
		perms: types.PermRWX,
	}

	sh := NewShell(v, "tester")
	ctx := context.Background()

	// Test absolute path
	resolved, err := sh.resolveCommand(ctx, "/bin/mycmd")
	if err != nil {
		t.Errorf("resolveCommand(/bin/mycmd) error: %v", err)
	}
	if resolved != "/bin/mycmd" {
		t.Errorf("resolveCommand(/bin/mycmd) = %q, want /bin/mycmd", resolved)
	}

	// Test command in PATH
	resolved, err = sh.resolveCommand(ctx, "mycmd")
	if err != nil {
		t.Errorf("resolveCommand(mycmd) error: %v", err)
	}
	if resolved != "/bin/mycmd" {
		t.Errorf("resolveCommand(mycmd) = %q, want /bin/mycmd", resolved)
	}

	// Test command not found
	_, err = sh.resolveCommand(ctx, "nonexistent")
	if err == nil {
		t.Error("resolveCommand(nonexistent) should fail")
	}
}

// ─── Cwd and setCwd Tests ───

func TestShellCwd(t *testing.T) {
	v := newMockVirtualOS()
	v.dirs["/"] = true
	v.dirs["/home"] = true
	v.dirs["/home/tester"] = true
	v.dirs["/tmp"] = true

	sh := NewShell(v, "tester")

	if sh.Cwd() != "/home/tester" {
		t.Errorf("initial Cwd = %q, want /home/tester", sh.Cwd())
	}

	sh.setCwd("/tmp")
	if sh.Cwd() != "/tmp" {
		t.Errorf("after setCwd, Cwd = %q, want /tmp", sh.Cwd())
	}
}
