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

func setupIntegration(t *testing.T) (*shellfish.VirtualOS, *shellfish.Shell) {
	t.Helper()
	v := shellfish.New()
	rootFS, err := shellfish.Configure(v)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	rootFS.AddDir("home/agent")
	rootFS.AddDir("home/agent/memory")
	rootFS.AddFile("home/agent/memory/facts.json", []byte(`{"name":"test-agent"}`), shellfish.PermRW)
	rootFS.AddFile("home/agent/memory/daily/2026-02-20.md", []byte("# Today\n- worked on shellfish tests\n"), shellfish.PermRW)

	sh := v.Shell("agent")
	return v, sh
}

func TestIntegrationFullWorkflow(t *testing.T) {
	_, sh := setupIntegration(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "pwd")
	if !strings.Contains(result.Output, "/home/agent") {
		t.Errorf("initial pwd = %q", result.Output)
	}

	result = sh.Execute(ctx, "ls ~")
	if !strings.Contains(result.Output, "memory") {
		t.Errorf("ls ~ should show memory/: %q", result.Output)
	}

	result = sh.Execute(ctx, "cat ~/memory/facts.json")
	if !strings.Contains(result.Output, "test-agent") {
		t.Errorf("cat facts.json: %q", result.Output)
	}

	result = sh.Execute(ctx, "write ~/memory/notes.md hello from integration test")
	if result.Code != 0 {
		t.Errorf("write failed: %q", result.Output)
	}

	result = sh.Execute(ctx, "cat ~/memory/notes.md")
	if !strings.Contains(result.Output, "hello from integration test") {
		t.Errorf("read back written file: %q", result.Output)
	}

	result = sh.Execute(ctx, "find ~ -name *.md")
	if !strings.Contains(result.Output, "notes.md") {
		t.Errorf("find should discover notes.md: %q", result.Output)
	}

	result = sh.Execute(ctx, "uname -a")
	if !strings.Contains(result.Output, "AgentFS") {
		t.Errorf("uname -a: %q", result.Output)
	}
}

func TestIntegrationPipelineChain(t *testing.T) {
	v, sh := setupIntegration(t)
	ctx := context.Background()

	v.Write(ctx, "/home/agent/lines.txt", strings.NewReader("alpha\nbeta\ngamma\ndelta\nepsilon\n"))

	result := sh.Execute(ctx, "cat ~/lines.txt | head -n 3 | tail -n 1")
	got := strings.TrimSpace(result.Output)
	if got != "gamma" {
		t.Errorf("cat | head -n 3 | tail -n 1 = %q, want gamma", got)
	}
}

func TestIntegrationRedirectionChain(t *testing.T) {
	v, sh := setupIntegration(t)
	ctx := context.Background()

	sh.Execute(ctx, "echo first line > ~/chain.txt")
	sh.Execute(ctx, "echo second line >> ~/chain.txt")
	sh.Execute(ctx, "echo third line >> ~/chain.txt")

	f, err := v.Open(ctx, "/home/agent/chain.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	content := string(data)
	if !strings.Contains(content, "first") || !strings.Contains(content, "second") || !strings.Contains(content, "third") {
		t.Errorf("chained redirects: %q", content)
	}
}

func TestIntegrationLogicalOperators(t *testing.T) {
	_, sh := setupIntegration(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "cat ~/memory/facts.json && echo success")
	if !strings.Contains(result.Output, "test-agent") || !strings.Contains(result.Output, "success") {
		t.Errorf("&& chain: %q", result.Output)
	}

	result = sh.Execute(ctx, "cat ~/nonexistent && echo should_not_appear")
	if strings.Contains(result.Output, "should_not_appear") {
		t.Error("&& should short-circuit on failure")
	}

	result = sh.Execute(ctx, "cat ~/nonexistent || echo fallback_worked")
	if !strings.Contains(result.Output, "fallback_worked") {
		t.Errorf("|| fallback: %q", result.Output)
	}
}

func TestIntegrationCommandGroupWithRedirect(t *testing.T) {
	v, sh := setupIntegration(t)
	ctx := context.Background()

	sh.Execute(ctx, "{ echo header; cat ~/memory/facts.json } > ~/combined.txt")

	f, err := v.Open(ctx, "/home/agent/combined.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	content := string(data)
	if !strings.Contains(content, "header") || !strings.Contains(content, "test-agent") {
		t.Errorf("command group redirect: %q", content)
	}
}

func TestIntegrationHereDoc(t *testing.T) {
	v, sh := setupIntegration(t)
	ctx := context.Background()

	heredoc := "write ~/heredoc.txt << END\nline one\nline two\nline three\nEND"
	sh.Execute(ctx, heredoc)

	f, err := v.Open(ctx, "/home/agent/heredoc.txt")
	if err != nil {
		t.Fatalf("Open heredoc.txt: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	content := string(data)
	if !strings.Contains(content, "line one") || !strings.Contains(content, "line three") {
		t.Errorf("heredoc content: %q", content)
	}
}

func TestIntegrationEnvironmentVariables(t *testing.T) {
	_, sh := setupIntegration(t)
	ctx := context.Background()

	sh.Env.Set("GREETING", "hello_shellfish")
	result := sh.Execute(ctx, "echo $GREETING")
	if !strings.Contains(result.Output, "hello_shellfish") {
		t.Errorf("env expansion: %q", result.Output)
	}

	result = sh.Execute(ctx, "echo ${HOME}")
	if !strings.Contains(result.Output, "/home/agent") {
		t.Errorf("braced env expansion: %q", result.Output)
	}
}

func TestIntegrationMultiMountSearch(t *testing.T) {
	v, sh := setupIntegration(t)

	local := mounts.NewLocalFS(t.TempDir(), shellfish.PermRW)
	v.Mount("/ext", local)

	ctx := context.Background()
	v.Write(ctx, "/ext/report.txt", strings.NewReader("quarterly report"))

	result := sh.Execute(ctx, "search report --scope /ext")
	_ = result
}

func TestIntegrationCdAndRelativePaths(t *testing.T) {
	_, sh := setupIntegration(t)
	ctx := context.Background()

	sh.Execute(ctx, "cd ~/memory")
	result := sh.Execute(ctx, "pwd")
	if !strings.Contains(result.Output, "/home/agent/memory") {
		t.Errorf("pwd after cd: %q", result.Output)
	}

	result = sh.Execute(ctx, "cat facts.json")
	if !strings.Contains(result.Output, "test-agent") {
		t.Errorf("relative cat: %q", result.Output)
	}
}

func TestIntegrationMkdirRmWorkflow(t *testing.T) {
	v, sh := setupIntegration(t)
	ctx := context.Background()

	sh.Execute(ctx, "mkdir ~/workspace")
	sh.Execute(ctx, "write ~/workspace/project.md # My Project")

	entry, err := v.Stat(ctx, "/home/agent/workspace/project.md")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.IsDir {
		t.Error("project.md should be file")
	}

	sh.Execute(ctx, "rm ~/workspace/project.md")
	_, err = v.Stat(ctx, "/home/agent/workspace/project.md")
	if err == nil {
		t.Error("project.md should be removed")
	}
}

func TestIntegrationMvWorkflow(t *testing.T) {
	v, sh := setupIntegration(t)
	ctx := context.Background()

	sh.Execute(ctx, "write ~/old_name.txt content here")
	sh.Execute(ctx, "mv ~/old_name.txt ~/new_name.txt")

	_, err := v.Stat(ctx, "/home/agent/old_name.txt")
	if err == nil {
		t.Error("old path should not exist")
	}
	f, err := v.Open(ctx, "/home/agent/new_name.txt")
	if err != nil {
		t.Fatalf("Open new_name.txt: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "content here") {
		t.Errorf("mv content = %q", string(data))
	}
}

func TestIntegrationProcVersion(t *testing.T) {
	_, sh := setupIntegration(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "cat /proc/version")
	if !strings.Contains(result.Output, "AgentFS") {
		t.Errorf("proc/version: %q", result.Output)
	}
}

func TestIntegrationWhichFindsBuiltins(t *testing.T) {
	_, sh := setupIntegration(t)
	ctx := context.Background()

	cmds := []string{"ls", "cat", "write", "stat", "head", "tail", "find", "mkdir", "rm", "mv", "mount", "which", "uname", "grep", "search"}
	for _, cmd := range cmds {
		result := sh.Execute(ctx, "which "+cmd)
		if result.Code != 0 {
			t.Errorf("which %s failed: %q", cmd, result.Output)
		}
		if !strings.Contains(result.Output, cmd) {
			t.Errorf("which %s should contain path: %q", cmd, result.Output)
		}
	}
}

func TestIntegrationMountList(t *testing.T) {
	_, sh := setupIntegration(t)
	ctx := context.Background()

	result := sh.Execute(ctx, "mount")
	if !strings.Contains(result.Output, "/") {
		t.Errorf("mount should list /: %q", result.Output)
	}
	if !strings.Contains(result.Output, "/proc") {
		t.Errorf("mount should list /proc: %q", result.Output)
	}
}
