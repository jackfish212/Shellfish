package shellfish_test

import (
	"context"
	"io"
	"strings"
	"testing"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/builtins"
	"github.com/jackfish212/shellfish/mounts"
)

func setupShell(t *testing.T) (*shellfish.Shell, *shellfish.VirtualOS) {
	t.Helper()
	v := shellfish.New()
	root := mounts.NewMemFS(shellfish.PermRW)
	v.Mount("/", root)
	root.AddDir("bin")
	root.AddDir("usr")
	root.AddDir("usr/bin")
	root.AddDir("etc")
	root.AddFile("etc/profile", []byte("export PATH=/usr/bin:/bin\n"), shellfish.PermRO)
	root.AddDir("home")
	root.AddDir("home/tester")
	root.AddFile("home/tester/hello.txt", []byte("hello world"), shellfish.PermRW)
	root.AddDir("tmp")

	builtins.RegisterBuiltinsOnFS(v, root)

	sh := v.Shell("tester")
	return sh, v
}

// ─── Basic Shell Builtins ───

func TestShellPwd(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "pwd")
	got := strings.TrimSpace(result.Output)
	if got != "/home/tester" {
		t.Errorf("pwd = %q, want /home/tester", got)
	}
}

func TestShellCd(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	sh.Execute(ctx, "cd /tmp")
	if sh.Cwd() != "/tmp" {
		t.Errorf("Cwd after cd = %q, want /tmp", sh.Cwd())
	}

	sh.Execute(ctx, "cd")
	if sh.Cwd() != "/home/tester" {
		t.Errorf("Cwd after cd (no args) = %q, want /home/tester", sh.Cwd())
	}
}

func TestShellCdNonexistent(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "cd /nonexistent")
	if result.Code == 0 {
		t.Error("cd to nonexistent dir should fail")
	}
}

func TestShellEcho(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo hello world")
	got := strings.TrimSpace(result.Output)
	if got != "hello world" {
		t.Errorf("echo = %q", got)
	}
}

func TestShellEnv(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "env")
	if !strings.Contains(result.Output, "USER=tester") {
		t.Errorf("env should contain USER=tester, got: %q", result.Output)
	}
}

func TestShellHistory(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	sh.Execute(ctx, "echo first")
	sh.Execute(ctx, "echo second")
	result := sh.Execute(ctx, "history")
	if !strings.Contains(result.Output, "echo first") {
		t.Errorf("history should contain 'echo first': %q", result.Output)
	}
}

func TestShellEmptyCommand(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "")
	if result.Output != "" || result.Code != 0 {
		t.Errorf("empty command should be no-op, got output=%q code=%d", result.Output, result.Code)
	}
}

// ─── Command Execution ───

func TestShellExecBuiltin(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "cat ~/hello.txt")
	got := strings.TrimSpace(result.Output)
	if got != "hello world" {
		t.Errorf("cat = %q, want %q", got, "hello world")
	}
}

func TestShellExecNotFound(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "nonexistent_command")
	if result.Code == 0 {
		t.Error("nonexistent command should have non-zero exit code")
	}
	if !strings.Contains(result.Output, "not found") {
		t.Errorf("should say 'not found': %q", result.Output)
	}
}

// ─── Pipes ───

func TestShellPipe(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	v.Write(ctx, "/home/tester/multiline.txt", strings.NewReader("line1\nline2\nline3\n"))

	result := sh.Execute(ctx, "cat ~/multiline.txt | head -n 2")
	lines := strings.Split(strings.TrimSpace(result.Output), "\n")
	if len(lines) != 2 {
		t.Errorf("pipe result should have 2 lines, got %d: %q", len(lines), result.Output)
	}
}

func TestShellMultiPipe(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo hello world | cat | cat")
	got := strings.TrimSpace(result.Output)
	if got != "hello world" {
		t.Errorf("multi pipe = %q", got)
	}
}

// ─── Redirections ───

func TestShellRedirectWrite(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	sh.Execute(ctx, "echo redirected > ~/output.txt")

	f, err := v.Open(ctx, "/home/tester/output.txt")
	if err != nil {
		t.Fatalf("Open output.txt: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "redirected") {
		t.Errorf("redirected content = %q", string(data))
	}
}

func TestShellRedirectAppend(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	sh.Execute(ctx, "echo line1 > ~/append.txt")
	sh.Execute(ctx, "echo line2 >> ~/append.txt")

	f, err := v.Open(ctx, "/home/tester/append.txt")
	if err != nil {
		t.Fatalf("Open append.txt: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	content := string(data)
	if !strings.Contains(content, "line1") || !strings.Contains(content, "line2") {
		t.Errorf("appended content = %q", content)
	}
}

// ─── Logical Operators ───

func TestShellLogicalAnd(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo first && echo second")
	if !strings.Contains(result.Output, "first") || !strings.Contains(result.Output, "second") {
		t.Errorf("&& should run both: %q", result.Output)
	}
}

func TestShellLogicalAndShortCircuit(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "nonexistent_cmd && echo should_not_run")
	if strings.Contains(result.Output, "should_not_run") {
		t.Error("&& should short-circuit on failure")
	}
}

func TestShellLogicalOr(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "nonexistent_cmd || echo fallback")
	if !strings.Contains(result.Output, "fallback") {
		t.Errorf("|| should run fallback: %q", result.Output)
	}
}

func TestShellLogicalOrSkip(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo success || echo should_not_run")
	if strings.Contains(result.Output, "should_not_run") {
		t.Error("|| should skip second when first succeeds")
	}
}

// ─── Command Groups ───

func TestShellCommandGroup(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "{ echo a; echo b }")
	if !strings.Contains(result.Output, "a") || !strings.Contains(result.Output, "b") {
		t.Errorf("command group output = %q", result.Output)
	}
}

func TestShellCommandGroupRedirect(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	sh.Execute(ctx, "{ echo line1; echo line2 } > ~/group.txt")

	f, err := v.Open(ctx, "/home/tester/group.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "line1") || !strings.Contains(string(data), "line2") {
		t.Errorf("group redirect = %q", string(data))
	}
}

// ─── Environment Variable Expansion ───

func TestShellEnvExpansion(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo $HOME")
	got := strings.TrimSpace(result.Output)
	if got != "/home/tester" {
		t.Errorf("$HOME expansion = %q", got)
	}
}

func TestShellEnvExpansionBraces(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo ${USER}")
	got := strings.TrimSpace(result.Output)
	if got != "tester" {
		t.Errorf("${USER} expansion = %q", got)
	}
}

func TestShellTildeExpansion(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "cat ~/hello.txt")
	if !strings.Contains(result.Output, "hello world") {
		t.Errorf("tilde expansion failed: %q", result.Output)
	}
}

// ─── Here-Documents ───

func TestShellHereDoc(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	heredoc := "write ~/heredoc.txt << EOF\nline1\nline2\nEOF"
	sh.Execute(ctx, heredoc)

	f, err := v.Open(ctx, "/home/tester/heredoc.txt")
	if err != nil {
		t.Fatalf("Open heredoc.txt: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "line1") || !strings.Contains(string(data), "line2") {
		t.Errorf("heredoc content = %q", string(data))
	}
}
