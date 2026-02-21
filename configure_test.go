package grasp

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestConfigure(t *testing.T) {
	v := New()
	rootFS, err := Configure(v)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if rootFS == nil {
		t.Fatal("Configure returned nil rootFS")
	}

	ctx := context.Background()

	dirs := []string{"/bin", "/usr/bin", "/etc", "/home", "/root", "/tmp", "/var", "/proc"}
	for _, dir := range dirs {
		entry, err := v.Stat(ctx, dir)
		if err != nil {
			t.Errorf("Stat(%q): %v", dir, err)
			continue
		}
		if !entry.IsDir {
			t.Errorf("%q should be a directory", dir)
		}
	}
}

func TestMountRootFS(t *testing.T) {
	v := New()
	rootFS, err := MountRootFS(v)
	if err != nil {
		t.Fatalf("MountRootFS: %v", err)
	}
	if rootFS == nil {
		t.Fatal("returned nil rootFS")
	}

	ctx := context.Background()
	entry, err := v.Stat(ctx, "/etc/profile")
	if err != nil {
		t.Fatalf("Stat /etc/profile: %v", err)
	}
	if entry.IsDir {
		t.Error("/etc/profile should be a file")
	}

	f, err := v.Open(ctx, "/etc/profile")
	if err != nil {
		t.Fatalf("Open /etc/profile: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "PATH") {
		t.Errorf("/etc/profile should contain PATH: %q", string(data))
	}
}

func TestProcProvider(t *testing.T) {
	v := New()
	MountRootFS(v)
	MountProc(v)

	ctx := context.Background()

	entry, err := v.Stat(ctx, "/proc")
	if err != nil {
		t.Fatalf("Stat /proc: %v", err)
	}
	if !entry.IsDir {
		t.Error("/proc should be directory")
	}

	entries, err := v.List(ctx, "/proc", ListOpts{})
	if err != nil {
		t.Fatalf("List /proc: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "version" {
			found = true
			break
		}
	}
	if !found {
		t.Error("/proc should contain 'version'")
	}

	f, err := v.Open(ctx, "/proc/version")
	if err != nil {
		t.Fatalf("Open /proc/version: %v", err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	content := string(data)
	if !strings.Contains(content, "AgentFS") {
		t.Errorf("/proc/version should contain AgentFS: %q", content)
	}
}

func TestProcProviderStatNotFound(t *testing.T) {
	p := NewProcProvider()
	ctx := context.Background()

	_, err := p.Stat(ctx, "/nonexistent")
	if err == nil {
		t.Error("Stat nonexistent proc file should fail")
	}
}

func TestProcProviderListNonRoot(t *testing.T) {
	p := NewProcProvider()
	ctx := context.Background()

	_, err := p.List(ctx, "/subdir", ListOpts{})
	if err == nil {
		t.Error("List non-root proc dir should fail")
	}
}

func TestProcProviderOpenNotFound(t *testing.T) {
	p := NewProcProvider()
	ctx := context.Background()

	_, err := p.Open(ctx, "/ghost")
	if err == nil {
		t.Error("Open nonexistent proc file should fail")
	}
}

func TestGetVersionInfo(t *testing.T) {
	v := GetVersionInfo()
	if v.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
	if v.Platform == "" {
		t.Error("Platform should not be empty")
	}
}

func TestVersionInfoProcVersion(t *testing.T) {
	v := GetVersionInfo()
	s := v.ProcVersion()
	if !strings.Contains(s, "AgentFS") {
		t.Errorf("ProcVersion should contain AgentFS: %q", s)
	}
	if !strings.Contains(s, "Go") {
		t.Errorf("ProcVersion should contain Go version: %q", s)
	}
}

// ─── Env context ───

func TestWithEnvAndEnv(t *testing.T) {
	ctx := context.Background()
	ctx = WithEnv(ctx, map[string]string{"FOO": "bar", "BAZ": "qux"})

	if got := Env(ctx, "FOO"); got != "bar" {
		t.Errorf("Env(FOO) = %q, want bar", got)
	}
	if got := Env(ctx, "BAZ"); got != "qux" {
		t.Errorf("Env(BAZ) = %q, want qux", got)
	}
	if got := Env(ctx, "MISSING"); got != "" {
		t.Errorf("Env(MISSING) = %q, want empty", got)
	}
}

func TestEnvWithoutContext(t *testing.T) {
	ctx := context.Background()
	if got := Env(ctx, "FOO"); got != "" {
		t.Errorf("Env without WithEnv = %q, want empty", got)
	}
}
