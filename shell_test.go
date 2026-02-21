package grasp_test

import (
	"context"
	"io"
	"strings"
	"testing"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mounts"
)

func setupShell(t *testing.T) (*grasp.Shell, *grasp.VirtualOS) {
	t.Helper()
	v := grasp.New()
	root := mounts.NewMemFS(grasp.PermRW)
	if err := v.Mount("/", root); err != nil {
		t.Fatal(err)
	}
	root.AddDir("bin")
	root.AddDir("usr")
	root.AddDir("usr/bin")
	root.AddDir("etc")
	root.AddFile("etc/profile", []byte("export PATH=/usr/bin:/bin\n"), grasp.PermRO)
	root.AddDir("home")
	root.AddDir("home/tester")
	root.AddFile("home/tester/hello.txt", []byte("hello world"), grasp.PermRW)
	root.AddDir("tmp")

	if err := builtins.RegisterBuiltinsOnFS(v, root); err != nil {
		t.Fatal(err)
	}

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

	if err := v.Write(ctx, "/home/tester/multiline.txt", strings.NewReader("line1\nline2\nline3\n")); err != nil {
		t.Fatal(err)
	}

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

// ─── Glob / Wildcard Expansion ───

func TestShellGlobStar(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	if err := v.Write(ctx, "/home/tester/a.txt", strings.NewReader("aaa")); err != nil {
		t.Fatal(err)
	}
	if err := v.Write(ctx, "/home/tester/b.txt", strings.NewReader("bbb")); err != nil {
		t.Fatal(err)
	}
	if err := v.Write(ctx, "/home/tester/c.log", strings.NewReader("ccc")); err != nil {
		t.Fatal(err)
	}

	result := sh.Execute(ctx, "echo *.txt")
	got := strings.TrimSpace(result.Output)
	if !strings.Contains(got, "a.txt") || !strings.Contains(got, "b.txt") {
		t.Errorf("*.txt should match a.txt and b.txt, got %q", got)
	}
	if strings.Contains(got, "c.log") {
		t.Errorf("*.txt should not match c.log, got %q", got)
	}
}

func TestShellGlobQuestion(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	if err := v.Write(ctx, "/home/tester/f1.txt", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if err := v.Write(ctx, "/home/tester/f2.txt", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if err := v.Write(ctx, "/home/tester/f10.txt", strings.NewReader("")); err != nil {
		t.Fatal(err)
	}

	result := sh.Execute(ctx, "echo f?.txt")
	got := strings.TrimSpace(result.Output)
	if !strings.Contains(got, "f1.txt") || !strings.Contains(got, "f2.txt") {
		t.Errorf("f?.txt should match f1.txt and f2.txt, got %q", got)
	}
	if strings.Contains(got, "f10.txt") {
		t.Errorf("f?.txt should not match f10.txt, got %q", got)
	}
}

func TestShellGlobWithDir(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	v.Write(ctx, "/tmp/x.go", strings.NewReader(""))
	v.Write(ctx, "/tmp/y.go", strings.NewReader(""))
	v.Write(ctx, "/tmp/z.md", strings.NewReader(""))

	result := sh.Execute(ctx, "echo /tmp/*.go")
	got := strings.TrimSpace(result.Output)
	if !strings.Contains(got, "/tmp/x.go") || !strings.Contains(got, "/tmp/y.go") {
		t.Errorf("/tmp/*.go should match x.go and y.go, got %q", got)
	}
	if strings.Contains(got, "z.md") {
		t.Errorf("/tmp/*.go should not match z.md, got %q", got)
	}
}

func TestShellGlobQuotedNoExpand(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	v.Write(ctx, "/home/tester/a.txt", strings.NewReader(""))

	result := sh.Execute(ctx, `echo "*.txt"`)
	got := strings.TrimSpace(result.Output)
	if got != "*.txt" {
		t.Errorf("quoted glob should not expand, got %q", got)
	}

	result2 := sh.Execute(ctx, "echo '*.txt'")
	got2 := strings.TrimSpace(result2.Output)
	if got2 != "*.txt" {
		t.Errorf("single-quoted glob should not expand, got %q", got2)
	}
}

func TestShellGlobNoMatch(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo *.zzz")
	got := strings.TrimSpace(result.Output)
	if got != "*.zzz" {
		t.Errorf("unmatched glob should be kept as-is, got %q", got)
	}
}

func TestShellGlobRelativeDir(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	v.Write(ctx, "/home/tester/sub/p.txt", strings.NewReader(""))
	v.Write(ctx, "/home/tester/sub/q.txt", strings.NewReader(""))

	result := sh.Execute(ctx, "echo sub/*.txt")
	got := strings.TrimSpace(result.Output)
	if !strings.Contains(got, "sub/p.txt") || !strings.Contains(got, "sub/q.txt") {
		t.Errorf("sub/*.txt should expand with relative prefix, got %q", got)
	}
}

func TestShellGlobWithPipe(t *testing.T) {
	sh, v := setupShell(t)
	ctx := context.Background()

	v.Write(ctx, "/home/tester/data1.txt", strings.NewReader("hello\n"))
	v.Write(ctx, "/home/tester/data2.txt", strings.NewReader("world\n"))

	result := sh.Execute(ctx, "cat *.txt | head -n 1")
	if result.Code != 0 {
		t.Errorf("glob with pipe should succeed, got code %d: %s", result.Code, result.Output)
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

// ─── Echo Options ───

func TestShellEchoNoNewline(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo -n hello")
	if strings.HasSuffix(result.Output, "\n") {
		t.Error("echo -n should not add trailing newline")
	}
	if result.Output != "hello" {
		t.Errorf("echo -n output = %q, want %q", result.Output, "hello")
	}
}

func TestShellEchoEscape(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, `echo -e "hello\tworld"`)
	if !strings.Contains(result.Output, "\t") {
		t.Errorf("echo -e should expand \\t, got: %q", result.Output)
	}

	result = sh.Execute(ctx, `echo -e "line1\nline2"`)
	if !strings.Contains(result.Output, "\nline2") {
		t.Errorf("echo -e should expand \\n, got: %q", result.Output)
	}
}

func TestShellEchoCombinedOptions(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, `echo -ne "hello\tworld"`)
	if strings.HasSuffix(result.Output, "\n") {
		t.Error("echo -ne should not add trailing newline")
	}
	if !strings.Contains(result.Output, "\t") {
		t.Errorf("echo -ne should expand \\t, got: %q", result.Output)
	}
}

// ─── Command Substitution ───

func TestShellCommandSubstitutionBacktick(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo `echo hello`")
	got := strings.TrimSpace(result.Output)
	if got != "hello" {
		t.Errorf("command substitution with backtick = %q, want %q", got, "hello")
	}
}

func TestShellCommandSubstitutionDollar(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo $(echo world)")
	got := strings.TrimSpace(result.Output)
	if got != "world" {
		t.Errorf("command substitution with $() = %q, want %q", got, "world")
	}
}

func TestShellCommandSubstitutionInString(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, `echo "prefix $(echo inner) suffix"`)
	got := strings.TrimSpace(result.Output)
	if got != "prefix inner suffix" {
		t.Errorf("nested command substitution = %q, want %q", got, "prefix inner suffix")
	}
}

func TestShellCommandSubstitutionPwd(t *testing.T) {
	sh, _ := setupShell(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "echo `pwd`")
	got := strings.TrimSpace(result.Output)
	if got != "/home/tester" {
		t.Errorf("pwd substitution = %q, want %q", got, "/home/tester")
	}
}
