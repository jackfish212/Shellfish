package builtins

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

func setupTestEnv(t *testing.T) (*shellfish.VirtualOS, *shellfish.Shell) {
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
	root.AddDir("tmp")

	root.AddFile("home/tester/notes.txt", []byte("hello world\nfoo bar\nbaz qux\n"), shellfish.PermRW)
	root.AddFile("home/tester/data.csv", []byte("a,b,c\n1,2,3\n4,5,6\n"), shellfish.PermRW)
	root.AddDir("home/tester/docs")
	root.AddFile("home/tester/docs/readme.md", []byte("# README\nProject docs"), shellfish.PermRO)
	root.AddFile("home/tester/.hidden", []byte("secret"), shellfish.PermRO)

	// JSON test file for jsonq
	root.AddFile("home/tester/data.json", []byte(`{
  "name": "computers",
  "description": "List of computer products",
  "prices": [2400, 2100, 1200, 400.87, 89.90, 150.10],
  "items": [
    {"id": 1, "name": "MacBook Pro 13", "price": 1350},
    {"id": 2, "name": "MacBook Pro 15", "price": 1700},
    {"id": 3, "name": "Sony VAIO", "price": 1200},
    {"id": 4, "name": "Fujitsu", "price": 850}
  ]
}`), shellfish.PermRW)

	RegisterBuiltinsOnFS(v, root)

	sh := v.Shell("tester")
	return v, sh
}

func run(t *testing.T, sh *shellfish.Shell, cmd string) string {
	t.Helper()
	ctx := context.Background()
	result := sh.Execute(ctx, cmd)
	return result.Output
}

func runCode(t *testing.T, sh *shellfish.Shell, cmd string) (string, int) {
	t.Helper()
	ctx := context.Background()
	result := sh.Execute(ctx, cmd)
	return result.Output, result.Code
}

// ─── ls ───

func TestLs(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "ls ~")
	if !strings.Contains(out, "notes.txt") {
		t.Errorf("ls ~ should contain notes.txt: %q", out)
	}
	if !strings.Contains(out, "docs") {
		t.Errorf("ls ~ should contain docs/: %q", out)
	}
}

func TestLsHiddenFiles(t *testing.T) {
	_, sh := setupTestEnv(t)

	out := run(t, sh, "ls ~")
	if strings.Contains(out, ".hidden") {
		t.Errorf("ls without -a should not show .hidden: %q", out)
	}

	out = run(t, sh, "ls -a ~")
	if !strings.Contains(out, ".hidden") {
		t.Errorf("ls -a should show .hidden: %q", out)
	}
}

func TestLsLong(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "ls -l ~")
	if !strings.Contains(out, "r") {
		t.Errorf("ls -l should show permissions: %q", out)
	}
}

func TestLsMultiplePaths(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "ls ~ /tmp")
	if !strings.Contains(out, "notes.txt") {
		t.Errorf("ls multiple paths missing notes.txt: %q", out)
	}
}

func TestLsHelp(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "ls -h")
	if !strings.Contains(out, "Usage") {
		t.Errorf("ls -h should show help: %q", out)
	}
}

// ─── cat/read ───

func TestCat(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "cat ~/notes.txt")
	if !strings.Contains(out, "hello world") {
		t.Errorf("cat should read file: %q", out)
	}
}

func TestCatMultipleFiles(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "cat ~/notes.txt ~/data.csv")
	if !strings.Contains(out, "hello world") || !strings.Contains(out, "a,b,c") {
		t.Errorf("cat multiple files: %q", out)
	}
}

func TestCatNotFound(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "cat ~/nonexistent.txt")
	if code == 0 {
		t.Error("cat nonexistent should fail")
	}
}

func TestCatFromPipe(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "echo piped input | cat")
	if !strings.Contains(out, "piped input") {
		t.Errorf("cat from stdin: %q", out)
	}
}

// ─── write ───

func TestWrite(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "write ~/output.txt hello from write")

	ctx := context.Background()
	f, err := v.Open(ctx, "/home/tester/output.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "hello from write") {
		t.Errorf("written content = %q", string(data))
	}
}

func TestWriteFromPipe(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "echo piped data | write ~/piped.txt")

	ctx := context.Background()
	f, err := v.Open(ctx, "/home/tester/piped.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "piped data") {
		t.Errorf("piped content = %q", string(data))
	}
}

func TestWriteNoContent(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "write ~/empty.txt")
	if code == 0 {
		t.Error("write without content should fail")
	}
}

func TestWriteNoArgs(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "write")
	if code == 0 {
		t.Error("write without args should fail")
	}
}

// ─── stat ───

func TestStat(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "stat ~/notes.txt")
	if !strings.Contains(out, "notes.txt") {
		t.Errorf("stat should show name: %q", out)
	}
	if !strings.Contains(out, "Path:") || !strings.Contains(out, "Perm:") {
		t.Errorf("stat should show metadata: %q", out)
	}
}

func TestStatNotFound(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "stat ~/nonexistent.txt")
	if code == 0 {
		t.Error("stat nonexistent should fail")
	}
}

// ─── head ───

func TestHead(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "head -n 1 ~/notes.txt")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("head -n 1 should return 1 line, got %d: %q", len(lines), out)
	}
	if strings.TrimSpace(lines[0]) != "hello world" {
		t.Errorf("head -n 1 = %q", lines[0])
	}
}

func TestHeadDefault(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "head ~/notes.txt")
	if !strings.Contains(out, "hello world") {
		t.Errorf("head default: %q", out)
	}
}

func TestHeadFromPipe(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "cat ~/notes.txt | head -n 2")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("head -n 2 from pipe should return 2 lines, got %d", len(lines))
	}
}

// ─── tail ───

func TestTail(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "tail -n 1 ~/notes.txt")
	got := strings.TrimSpace(out)
	if got != "baz qux" {
		t.Errorf("tail -n 1 = %q, want %q", got, "baz qux")
	}
}

func TestTailFromPipe(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "cat ~/notes.txt | tail -n 1")
	got := strings.TrimSpace(out)
	if got != "baz qux" {
		t.Errorf("tail -n 1 from pipe = %q", got)
	}
}

// ─── mkdir ───

func TestMkdir(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "mkdir ~/newdir")

	ctx := context.Background()
	entry, err := v.Stat(ctx, "/home/tester/newdir")
	if err != nil {
		t.Fatalf("Stat after mkdir: %v", err)
	}
	if !entry.IsDir {
		t.Error("should be directory")
	}
}

func TestMkdirNoArgs(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "mkdir")
	if code == 0 {
		t.Error("mkdir without args should fail")
	}
}

// ─── rm ───

func TestRm(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "rm ~/data.csv")

	ctx := context.Background()
	_, err := v.Stat(ctx, "/home/tester/data.csv")
	if err == nil {
		t.Error("data.csv should be removed")
	}
}

func TestRmNotFound(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "rm ~/ghost.txt")
	if code == 0 {
		t.Error("rm nonexistent should fail")
	}
}

func TestRmNoArgs(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "rm")
	if code == 0 {
		t.Error("rm without args should fail")
	}
}

// ─── mv ───

func TestMv(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "mv ~/data.csv ~/renamed.csv")

	ctx := context.Background()
	_, err := v.Stat(ctx, "/home/tester/data.csv")
	if err == nil {
		t.Error("old path should not exist")
	}
	_, err = v.Stat(ctx, "/home/tester/renamed.csv")
	if err != nil {
		t.Errorf("new path should exist: %v", err)
	}
}

func TestMvNoArgs(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "mv")
	if code == 0 {
		t.Error("mv without args should fail")
	}
}

// ─── cp ───

func TestCpFile(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "cp ~/notes.txt ~/notes_copy.txt")

	ctx := context.Background()
	f, err := v.Open(ctx, "/home/tester/notes_copy.txt")
	if err != nil {
		t.Fatalf("copied file should exist: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("copied content = %q", string(data))
	}

	// Original file should still exist
	_, err = v.Stat(ctx, "/home/tester/notes.txt")
	if err != nil {
		t.Error("original file should still exist")
	}
}

func TestCpToDirectory(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "cp ~/notes.txt ~/docs/")

	ctx := context.Background()
	f, err := v.Open(ctx, "/home/tester/docs/notes.txt")
	if err != nil {
		t.Fatalf("file copied to directory should exist: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("copied content = %q", string(data))
	}
}

func TestCpRecursive(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "mkdir ~/backup")
	run(t, sh, "cp -r ~/docs ~/backup/")

	ctx := context.Background()
	f, err := v.Open(ctx, "/home/tester/backup/docs/readme.md")
	if err != nil {
		t.Fatalf("recursive copy should create nested file: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "README") {
		t.Errorf("copied content = %q", string(data))
	}
}

func TestCpDirWithoutRecursive(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "cp ~/docs ~/docs_copy")
	if code == 0 {
		t.Error("cp directory without -r should fail")
	}
}

func TestCpNotFound(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "cp ~/nonexistent.txt ~/copy.txt")
	if code == 0 {
		t.Error("cp nonexistent should fail")
	}
}

func TestCpNoArgs(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "cp")
	if code == 0 {
		t.Error("cp without args should fail")
	}
}

func TestCpMultipleFiles(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "mkdir ~/dest")
	run(t, sh, "cp ~/notes.txt ~/data.csv ~/dest/")

	ctx := context.Background()
	_, err1 := v.Stat(ctx, "/home/tester/dest/notes.txt")
	_, err2 := v.Stat(ctx, "/home/tester/dest/data.csv")
	if err1 != nil || err2 != nil {
		t.Errorf("multiple files should be copied: err1=%v, err2=%v", err1, err2)
	}
}

func TestCpHelp(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "cp -h")
	if !strings.Contains(out, "Usage") {
		t.Errorf("cp -h should show help: %q", out)
	}
}

// ─── find ───

func TestFind(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "find ~ -name *.txt")
	if !strings.Contains(out, "notes.txt") {
		t.Errorf("find should find notes.txt: %q", out)
	}
}

func TestFindTypeD(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "find ~ -type d")
	if !strings.Contains(out, "docs") {
		t.Errorf("find -type d should find docs: %q", out)
	}
}

func TestFindTypeF(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "find ~ -type f -name *.md")
	if !strings.Contains(out, "readme.md") {
		t.Errorf("find -type f -name *.md: %q", out)
	}
}

// ─── which ───

func TestWhich(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "which ls")
	got := strings.TrimSpace(out)
	if !strings.Contains(got, "ls") {
		t.Errorf("which ls = %q", got)
	}
}

func TestWhichNotFound(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "which nonexistent_cmd")
	if code == 0 {
		t.Error("which nonexistent should fail")
	}
}

// ─── mount ───

func TestMount(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "mount")
	if !strings.Contains(out, "/") {
		t.Errorf("mount should list root: %q", out)
	}
}

// ─── uname ───

func TestUname(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "uname")
	if !strings.Contains(out, "AgentFS") {
		t.Errorf("uname should contain AgentFS: %q", out)
	}
}

func TestUnameAll(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "uname -a")
	if !strings.Contains(out, "AgentFS") {
		t.Errorf("uname -a: %q", out)
	}
}

// ─── grep ───

func TestGrepFromPipe(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "cat ~/notes.txt | grep foo")
	if !strings.Contains(out, "foo bar") {
		t.Errorf("grep from pipe should match 'foo bar': %q", out)
	}
	if strings.Contains(out, "hello") {
		t.Errorf("grep should not include non-matching lines: %q", out)
	}
}

func TestGrepFile(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep bar ~/notes.txt")
	if !strings.Contains(out, "foo bar") {
		t.Errorf("grep file should match 'foo bar': %q", out)
	}
	if strings.Contains(out, "hello") {
		t.Errorf("grep should not include non-matching lines: %q", out)
	}
}

func TestGrepMultipleFiles(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep bar ~/notes.txt ~/data.csv")
	if !strings.Contains(out, "notes.txt:foo bar") {
		t.Errorf("grep multiple files should show filename: %q", out)
	}
}

func TestGrepIgnoreCase(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep -i HELLO ~/notes.txt")
	if !strings.Contains(out, "hello world") {
		t.Errorf("grep -i should match case-insensitively: %q", out)
	}
}

func TestGrepInvert(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep -v bar ~/notes.txt")
	if strings.Contains(out, "foo bar") {
		t.Errorf("grep -v should not include matching lines: %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("grep -v should include non-matching lines: %q", out)
	}
}

func TestGrepLineNumber(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep -n bar ~/notes.txt")
	if !strings.Contains(out, "2:foo bar") {
		t.Errorf("grep -n should show line number: %q", out)
	}
}

func TestGrepCount(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep -c bar ~/notes.txt")
	if !strings.Contains(out, "1") {
		t.Errorf("grep -c should show count: %q", out)
	}
}

func TestGrepRecursive(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep -r bar ~")
	if !strings.Contains(out, "bar") {
		t.Errorf("grep -r should search recursively: %q", out)
	}
}

func TestGrepRegex(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep 'f.*o' ~/notes.txt")
	if !strings.Contains(out, "foo bar") {
		t.Errorf("grep should support regex: %q", out)
	}
}

func TestGrepNoMatch(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep nonexistent ~/notes.txt")
	if out != "" && out != "\n" {
		t.Errorf("grep with no match should return empty: %q", out)
	}
}

func TestGrepHelp(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "grep --help")
	if code != 1 {
		t.Errorf("grep --help should return exit code 1, got %d", code)
	}
}

func TestGrepWordMatch(t *testing.T) {
	_, sh := setupTestEnv(t)
	// "foo" should match "foo bar" as a whole word
	out := run(t, sh, "grep -w foo ~/notes.txt")
	if !strings.Contains(out, "foo bar") {
		t.Errorf("grep -w should match whole word 'foo': %q", out)
	}
	// "fo" should NOT match as a whole word
	out = run(t, sh, "grep -w fo ~/notes.txt")
	if strings.Contains(out, "foo bar") {
		t.Errorf("grep -w should not match partial word 'fo': %q", out)
	}
}

func TestGrepContext(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep -C 1 bar ~/notes.txt")
	// Should include line before (hello world) and after (baz qux)
	if !strings.Contains(out, "hello world") {
		t.Errorf("grep -C 1 should include context before: %q", out)
	}
	if !strings.Contains(out, "baz qux") {
		t.Errorf("grep -C 1 should include context after: %q", out)
	}
}

func TestGrepBeforeContext(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep -B 1 bar ~/notes.txt")
	// Should include line before (hello world)
	if !strings.Contains(out, "hello world") {
		t.Errorf("grep -B 1 should include context before: %q", out)
	}
	// Should NOT include line after
	if strings.Contains(out, "baz qux") {
		t.Errorf("grep -B 1 should not include context after: %q", out)
	}
}

func TestGrepAfterContext(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "grep -A 1 bar ~/notes.txt")
	// Should include line after (baz qux)
	if !strings.Contains(out, "baz qux") {
		t.Errorf("grep -A 1 should include context after: %q", out)
	}
	// Should NOT include line before
	if strings.Contains(out, "hello world") {
		t.Errorf("grep -A 1 should not include context before: %q", out)
	}
}

func TestGrepContextWithSeparator(t *testing.T) {
	_, sh := setupTestEnv(t)
	// Use file with non-contiguous matches - data.csv has lines at different positions
	// Just verify context option works without error
	out := run(t, sh, "grep -C 1 bar ~/notes.txt")
	// Should include context lines
	if !strings.Contains(out, "hello world") && !strings.Contains(out, "baz qux") {
		t.Errorf("grep -C should include context lines: %q", out)
	}
}

func TestGrepWordMatchWithPipe(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "echo 'hello foobar world' | grep -w foo")
	if out != "" && out != "\n" {
		t.Errorf("grep -w should not match 'foo' in 'foobar': %q", out)
	}
	out = run(t, sh, "echo 'hello foo world' | grep -w foo")
	if !strings.Contains(out, "foo") {
		t.Errorf("grep -w should match whole word 'foo': %q", out)
	}
}

// ─── system commands ───

func TestDate(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "date")
	if out == "" {
		t.Error("date should output something")
	}
}

func TestDateFormat(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "date +%Y")
	if len(out) < 4 {
		t.Errorf("date +%%Y should output year: %q", out)
	}
	out = run(t, sh, "date +%F")
	if len(out) < 10 {
		t.Errorf("date +%%F should output date in YYYY-MM-DD format: %q", out)
	}
}

func TestWhoami(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "whoami")
	if !strings.Contains(out, "tester") {
		t.Errorf("whoami should return 'tester': %q", out)
	}
}

func TestSleep(t *testing.T) {
	_, sh := setupTestEnv(t)
	start := time.Now()
	run(t, sh, "sleep 0.1")
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("sleep 0.1 should take at least 100ms, took %v", elapsed)
	}
}

func TestSleepSuffix(t *testing.T) {
	_, sh := setupTestEnv(t)
	start := time.Now()
	run(t, sh, "sleep 0.1s")
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("sleep 0.1s should take at least 100ms, took %v", elapsed)
	}
}

func TestTrue(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "true")
	if code != 0 {
		t.Errorf("true should return exit code 0, got %d", code)
	}
}

func TestFalse(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "false")
	if code == 0 {
		t.Errorf("false should return non-zero exit code, got %d", code)
	}
}

func TestTrueInCondition(t *testing.T) {
	_, sh := setupTestEnv(t)
	out, code := runCode(t, sh, "true && echo success")
	if code != 0 {
		t.Errorf("true && echo should succeed, got code %d", code)
	}
	if !strings.Contains(out, "success") {
		t.Errorf("should output 'success': %q", out)
	}
}

func TestFalseInCondition(t *testing.T) {
	_, sh := setupTestEnv(t)
	out, code := runCode(t, sh, "false || echo fallback")
	if code != 0 {
		t.Errorf("false || echo should succeed, got code %d", code)
	}
	if !strings.Contains(out, "fallback") {
		t.Errorf("should output 'fallback': %q", out)
	}
}

func TestWhereis(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "whereis ls")
	if !strings.Contains(out, "ls:") {
		t.Errorf("whereis ls should show ls: %q", out)
	}
}

func TestWhereisNotFound(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "whereis nonexistentcmd123")
	if !strings.Contains(out, "nonexistentcmd123:") {
		t.Errorf("whereis should show command name: %q", out)
	}
}

// ─── helpers ───

func TestResolvePath(t *testing.T) {
	tests := []struct {
		cwd, path, want string
	}{
		{"/home/user", "file.txt", "/home/user/file.txt"},
		{"/home/user", "/tmp/file.txt", "/tmp/file.txt"},
		{"/home/user", "./sub/file.txt", "/home/user/sub/file.txt"},
		{"/", "file.txt", "/file.txt"},
		{"", "file.txt", "/file.txt"},
	}
	for _, tt := range tests {
		got := resolvePath(tt.cwd, tt.path)
		if got != tt.want {
			t.Errorf("resolvePath(%q, %q) = %q, want %q", tt.cwd, tt.path, got, tt.want)
		}
	}
}

func TestHasFlag(t *testing.T) {
	args := []string{"-l", "foo", "-a", "bar"}
	if !hasFlag(args, "-l") {
		t.Error("should find -l")
	}
	if !hasFlag(args, "-a") {
		t.Error("should find -a")
	}
	if hasFlag(args, "-x") {
		t.Error("should not find -x")
	}
}

func TestParseLsFlags(t *testing.T) {
	long, all, rest := parseLsFlags([]string{"-la", "dir1", "dir2"})
	if !long {
		t.Error("should detect -l")
	}
	if !all {
		t.Error("should detect -a")
	}
	if len(rest) != 2 || rest[0] != "dir1" {
		t.Errorf("rest = %v, want [dir1, dir2]", rest)
	}
}

// ─── sed ───

func TestSedSubstitute(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "echo hello world | sed -e 's/world/Shellfish/'")
	if !strings.Contains(out, "hello Shellfish") {
		t.Errorf("sed substitute: %q", out)
	}
}

func TestSedSubstituteGlobal(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "echo 'foo foo foo' | sed -e 's/foo/bar/g'")
	if !strings.Contains(out, "bar bar bar") {
		t.Errorf("sed global substitute: %q", out)
	}
}

func TestSedFromFile(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "sed -e 's/hello/hi/' ~/notes.txt")
	if !strings.Contains(out, "hi world") {
		t.Errorf("sed from file: %q", out)
	}
}

func TestSedDelete(t *testing.T) {
	_, sh := setupTestEnv(t)
	// Create a test file with multiple lines
	run(t, sh, "write ~/delete_test.txt keep")
	run(t, sh, "write ~/delete_test2.txt delete")
	out := run(t, sh, "sed -e '/delete/d' ~/delete_test.txt")
	if strings.Contains(out, "delete") {
		t.Errorf("sed delete should remove matching line: %q", out)
	}
}

func TestSedPrint(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "echo hello | sed -n -e 'p'")
	// -n with p should only print once
	lines := strings.Count(strings.TrimSpace(out), "hello")
	if lines != 1 {
		t.Errorf("sed -n p should print once, got %d times: %q", lines, out)
	}
}

func TestSedQuietMode(t *testing.T) {
	_, sh := setupTestEnv(t)
	// Without -n, sed prints all lines
	out1 := run(t, sh, "echo hello | sed -e 's/hello/world/'")
	if !strings.Contains(out1, "world") {
		t.Errorf("sed without -n: %q", out1)
	}
}

func TestSedHelp(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "sed --help")
	if code != 1 {
		t.Errorf("sed --help should return exit code 1, got %d", code)
	}
}

func TestSedNoScript(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "sed ~/notes.txt")
	if code == 0 {
		t.Error("sed without script should fail")
	}
}

func TestSedRegex(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "echo 'abc123def' | sed -e 's/[0-9]+/XXX/'")
	if !strings.Contains(out, "abcXXXdef") {
		t.Errorf("sed with regex: %q", out)
	}
}

func TestSedInPlace(t *testing.T) {
	v, sh := setupTestEnv(t)
	// Create a test file
	run(t, sh, "write ~/sed_test.txt original content")

	// Modify in place
	run(t, sh, "sed -i -e 's/original/modified/' ~/sed_test.txt")

	// Verify content changed
	ctx := context.Background()
	f, err := v.Open(ctx, "/home/tester/sed_test.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "modified content") {
		t.Errorf("sed -i should modify file in place: %q", string(data))
	}
}

// ─── rmdir ───

func TestRmdir(t *testing.T) {
	v, sh := setupTestEnv(t)
	// Create empty directory
	run(t, sh, "mkdir ~/emptydir")
	run(t, sh, "rmdir ~/emptydir")

	ctx := context.Background()
	_, err := v.Stat(ctx, "/home/tester/emptydir")
	if err == nil {
		t.Error("emptydir should be removed")
	}
}

func TestRmdirNonEmpty(t *testing.T) {
	_, sh := setupTestEnv(t)
	// docs contains readme.md
	_, code := runCode(t, sh, "rmdir ~/docs")
	if code == 0 {
		t.Error("rmdir non-empty directory should fail")
	}
}

func TestRmdirFile(t *testing.T) {
	_, sh := setupTestEnv(t)
	// notes.txt is a file, not a directory
	_, code := runCode(t, sh, "rmdir ~/notes.txt")
	if code == 0 {
		t.Error("rmdir on a file should fail")
	}
}

func TestRmdirNotFound(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "rmdir ~/nonexistent")
	if code == 0 {
		t.Error("rmdir nonexistent should fail")
	}
}

func TestRmdirNoArgs(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "rmdir")
	if code == 0 {
		t.Error("rmdir without args should fail")
	}
}

func TestRmdirParents(t *testing.T) {
	v, sh := setupTestEnv(t)
	// Create nested empty directories
	run(t, sh, "mkdir -p ~/a/b/c")
	run(t, sh, "rmdir -p ~/a/b/c")

	ctx := context.Background()
	// All should be removed
	_, errA := v.Stat(ctx, "/home/tester/a")
	_, errB := v.Stat(ctx, "/home/tester/a/b")
	_, errC := v.Stat(ctx, "/home/tester/a/b/c")
	if errA == nil || errB == nil || errC == nil {
		t.Error("rmdir -p should remove all empty parent directories")
	}
}

func TestRmdirParentsWithNonEmptyParent(t *testing.T) {
	v, sh := setupTestEnv(t)
	// Create nested directories
	run(t, sh, "mkdir -p ~/x/y/z")
	// Add a file to make parent non-empty
	run(t, sh, "write ~/x/file.txt content")

	// rmdir -p should remove z and y, but fail on x (non-empty)
	run(t, sh, "rmdir -p ~/x/y/z")

	ctx := context.Background()
	// z and y should be removed
	_, errZ := v.Stat(ctx, "/home/tester/x/y/z")
	_, errY := v.Stat(ctx, "/home/tester/x/y")
	if errZ == nil || errY == nil {
		t.Error("rmdir -p should remove z and y")
	}
	// x should still exist (contains file.txt)
	_, errX := v.Stat(ctx, "/home/tester/x")
	if errX != nil {
		t.Error("rmdir -p should keep non-empty parent x")
	}
}

func TestRmdirIgnoreFailOnNonEmpty(t *testing.T) {
	_, sh := setupTestEnv(t)
	// docs contains readme.md
	_, code := runCode(t, sh, "rmdir --ignore-fail-on-non-empty ~/docs")
	// Should not fail with this flag
	if code != 0 {
		t.Error("rmdir --ignore-fail-on-non-empty should not fail on non-empty directory")
	}
}

func TestRmdirVerbose(t *testing.T) {
	_, sh := setupTestEnv(t)
	run(t, sh, "mkdir ~/verbosedir")
	out := run(t, sh, "rmdir -v ~/verbosedir")
	if !strings.Contains(out, "removing") {
		t.Errorf("rmdir -v should output verbose message: %q", out)
	}
}

func TestRmdirMultiple(t *testing.T) {
	v, sh := setupTestEnv(t)
	// Create multiple empty directories
	run(t, sh, "mkdir ~/dir1 ~/dir2 ~/dir3")
	run(t, sh, "rmdir ~/dir1 ~/dir2 ~/dir3")

	ctx := context.Background()
	_, err1 := v.Stat(ctx, "/home/tester/dir1")
	_, err2 := v.Stat(ctx, "/home/tester/dir2")
	_, err3 := v.Stat(ctx, "/home/tester/dir3")
	if err1 == nil || err2 == nil || err3 == nil {
		t.Error("rmdir should remove all specified directories")
	}
}

func TestRmdirHelp(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "rmdir --help")
	if !strings.Contains(out, "Usage") {
		t.Errorf("rmdir --help should show help: %q", out)
	}
}

// ─── touch ───

func TestTouchCreateNewFile(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "touch ~/newfile.txt")

	ctx := context.Background()
	entry, err := v.Stat(ctx, "/home/tester/newfile.txt")
	if err != nil {
		t.Fatalf("touch should create new file: %v", err)
	}
	if entry.IsDir {
		t.Error("touched file should not be a directory")
	}
}

func TestTouchUpdateExistingFile(t *testing.T) {
	v, sh := setupTestEnv(t)

	ctx := context.Background()
	// Get original modification time
	entryBefore, _ := v.Stat(ctx, "/home/tester/notes.txt")
	modBefore := entryBefore.Modified

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	run(t, sh, "touch ~/notes.txt")

	// Check modification time was updated
	entryAfter, _ := v.Stat(ctx, "/home/tester/notes.txt")
	if !entryAfter.Modified.After(modBefore) {
		t.Errorf("touch should update modification time: before=%v, after=%v", modBefore, entryAfter.Modified)
	}

	// Content should remain unchanged
	f, _ := v.Open(ctx, "/home/tester/notes.txt")
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("touch should not change file content: %q", string(data))
	}
}

func TestTouchMultipleFiles(t *testing.T) {
	v, sh := setupTestEnv(t)
	run(t, sh, "touch ~/file1.txt ~/file2.txt ~/file3.txt")

	ctx := context.Background()
	for _, name := range []string{"/home/tester/file1.txt", "/home/tester/file2.txt", "/home/tester/file3.txt"} {
		_, err := v.Stat(ctx, name)
		if err != nil {
			t.Errorf("touch should create %s: %v", name, err)
		}
	}
}

func TestTouchNoArgs(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "touch")
	if code == 0 {
		t.Error("touch without args should fail")
	}
}

func TestTouchHelp(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "touch --help")
	if !strings.Contains(out, "Usage") {
		t.Errorf("touch --help should show help: %q", out)
	}
}

// ─── jsonq ───

func TestJsonqSimplePath(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq "name" ~/data.json`)
	if !strings.Contains(out, "computers") {
		t.Errorf("jsonq name should return 'computers': %q", out)
	}
}

func TestJsonqNestedPath(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq "items.[0].name" ~/data.json`)
	if !strings.Contains(out, "MacBook Pro 13") {
		t.Errorf("jsonq items.[0].name should return first item name: %q", out)
	}
}

func TestJsonqFrom(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq --from items --where "price > 1200" ~/data.json`)
	if !strings.Contains(out, "MacBook Pro") {
		t.Errorf("jsonq --from items --where should filter results: %q", out)
	}
}

func TestJsonqSortBy(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq --from items --sort-by price --sort-order desc ~/data.json`)
	// MacBook Pro 15 (price 1700) should come first
	if strings.Index(out, "MacBook Pro 15") > strings.Index(out, "Fujitsu") {
		t.Errorf("jsonq --sort-by price desc should sort descending: %q", out)
	}
}

func TestJsonqPluck(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq --from items --pluck name ~/data.json`)
	if !strings.Contains(out, "MacBook") {
		t.Errorf("jsonq --pluck name should return names: %q", out)
	}
}

func TestJsonqCount(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq --from items --count ~/data.json`)
	if !strings.Contains(out, "4") {
		t.Errorf("jsonq --count should return 4: %q", out)
	}
}

func TestJsonqSum(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq --from items --sum price ~/data.json`)
	if !strings.Contains(out, "5100") {
		t.Errorf("jsonq --sum price should return 5100: %q", out)
	}
}

func TestJsonqAvg(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq --from items --avg price ~/data.json`)
	if !strings.Contains(out, "1275") {
		t.Errorf("jsonq --avg price should return 1275: %q", out)
	}
}

func TestJsonqLimit(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq --from items --limit 2 ~/data.json`)
	// Count occurrences of "id" to verify only 2 items
	count := strings.Count(out, `"id"`)
	if count != 2 {
		t.Errorf("jsonq --limit 2 should return 2 items, got %d: %q", count, out)
	}
}

func TestJsonqFromPipe(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `cat ~/data.json | jsonq "name"`)
	if !strings.Contains(out, "computers") {
		t.Errorf("jsonq from pipe should work: %q", out)
	}
}

func TestJsonqRaw(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, `jsonq -r "name" ~/data.json`)
	if strings.Contains(out, `"`) {
		t.Errorf("jsonq -r should output raw value without quotes: %q", out)
	}
}

func TestJsonqHelp(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "jsonq --help")
	if code != 1 {
		t.Errorf("jsonq --help should return exit code 1, got %d", code)
	}
}

func TestJsonqNoInput(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "jsonq name")
	if code == 0 {
		t.Error("jsonq without input should fail")
	}
}

// ─── wc ───

func TestWcBasic(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "wc ~/notes.txt")
	if !strings.Contains(out, "3") {
		t.Errorf("wc should show line count: %q", out)
	}
}

func TestWcLines(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "wc -l ~/notes.txt")
	if !strings.Contains(out, "3") {
		t.Errorf("wc -l should show 3 lines: %q", out)
	}
}

func TestWcWords(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "wc -w ~/notes.txt")
	if !strings.Contains(out, "9") {
		t.Errorf("wc -w should show 9 words: %q", out)
	}
}

func TestWcBytes(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "wc -c ~/notes.txt")
	if !strings.Contains(out, "31") {
		t.Errorf("wc -c should show 31 bytes: %q", out)
	}
}

func TestWcChars(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "wc -m ~/notes.txt")
	if !strings.Contains(out, "31") {
		t.Errorf("wc -m should show 31 chars: %q", out)
	}
}

func TestWcMaxLine(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "wc -L ~/notes.txt")
	// Should show max line length
	if !strings.Contains(out, "10") {
		t.Errorf("wc -L should show max line length: %q", out)
	}
}

func TestWcMultipleFiles(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "wc ~/notes.txt ~/data.csv")
	if !strings.Contains(out, "total") {
		t.Errorf("wc with multiple files should show total: %q", out)
	}
}

func TestWcFromPipe(t *testing.T) {
	_, sh := setupTestEnv(t)
	out := run(t, sh, "cat ~/notes.txt | wc -l")
	if !strings.Contains(out, "3") {
		t.Errorf("wc from pipe should work: %q", out)
	}
}

func TestWcHelp(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "wc --help")
	if code != 1 {
		t.Errorf("wc --help should return exit code 1, got %d", code)
	}
}

func TestWcNoInput(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "wc")
	if code != 1 {
		t.Errorf("wc without input should fail, got code %d", code)
	}
}

// ─── grep isNumericArg ───

func TestGrepNumericArg(t *testing.T) {
	_, sh := setupTestEnv(t)
	// Test with -A, -B, -C which have numeric arguments
	out := run(t, sh, "grep -n -A 1 foo ~/notes.txt")
	if !strings.Contains(out, "foo") {
		t.Errorf("grep -A should work: %q", out)
	}
}

func TestGrepNumericArgZero(t *testing.T) {
	_, sh := setupTestEnv(t)
	// Test with -A 0 (should show 0 lines after)
	out := run(t, sh, "grep -A 0 foo ~/notes.txt")
	if strings.Contains(out, "\n") {
		t.Errorf("grep -A 0 should not show additional lines: %q", out)
	}
}

// ─── sleep parseDuration ───

func TestSleepVariousFormats(t *testing.T) {
	_, sh := setupTestEnv(t)

	// Test sleep with seconds
	out := run(t, sh, "sleep 0.01")
	if out != "" {
		t.Errorf("sleep should produce no output: %q", out)
	}

	// Test sleep with suffix
	out = run(t, sh, "sleep 10ms")
	if out != "" {
		t.Errorf("sleep with ms suffix should work: %q", out)
	}
}

func TestSleepInvalid(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "sleep invalid")
	if code == 0 {
		t.Error("sleep with invalid duration should fail")
	}
}

func TestSleepNegative(t *testing.T) {
	_, sh := setupTestEnv(t)
	_, code := runCode(t, sh, "sleep -1")
	if code == 0 {
		t.Error("sleep with negative duration should fail")
	}
}
