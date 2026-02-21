package types

import (
	"context"
	"io"
	"strings"
	"testing"
)

// ─── Perm ───

func TestPermBits(t *testing.T) {
	tests := []struct {
		perm              Perm
		read, write, exec bool
		str               string
	}{
		{PermNone, false, false, false, "---"},
		{PermRO, true, false, false, "r--"},
		{PermRW, true, true, false, "rw-"},
		{PermRX, true, false, true, "r-x"},
		{PermRWX, true, true, true, "rwx"},
		{PermWrite, false, true, false, "-w-"},
		{PermExec, false, false, true, "--x"},
		{PermWrite | PermExec, false, true, true, "-wx"},
	}
	for _, tt := range tests {
		if tt.perm.CanRead() != tt.read {
			t.Errorf("Perm(%d).CanRead() = %v, want %v", tt.perm, tt.perm.CanRead(), tt.read)
		}
		if tt.perm.CanWrite() != tt.write {
			t.Errorf("Perm(%d).CanWrite() = %v, want %v", tt.perm, tt.perm.CanWrite(), tt.write)
		}
		if tt.perm.CanExec() != tt.exec {
			t.Errorf("Perm(%d).CanExec() = %v, want %v", tt.perm, tt.perm.CanExec(), tt.exec)
		}
		if got := tt.perm.String(); got != tt.str {
			t.Errorf("Perm(%d).String() = %q, want %q", tt.perm, got, tt.str)
		}
	}
}

// ─── Entry ───

func TestEntryString(t *testing.T) {
	e := Entry{Name: "hello.txt", Perm: PermRO}
	got := e.String()
	if !strings.Contains(got, "hello.txt") {
		t.Errorf("Entry.String() missing file name: %q", got)
	}
	if !strings.HasPrefix(got, "-") {
		t.Errorf("Entry.String() should start with '-' for file: %q", got)
	}

	d := Entry{Name: "docs", IsDir: true, Perm: PermRX}
	got = d.String()
	if !strings.HasPrefix(got, "d") {
		t.Errorf("Entry.String() should start with 'd' for dir: %q", got)
	}
	if !strings.Contains(got, "docs/") {
		t.Errorf("Entry.String() should append '/' for dir: %q", got)
	}
}

func TestEntryStringWithMeta(t *testing.T) {
	e := Entry{
		Name: "search",
		Perm: PermRX,
		Meta: map[string]string{"kind": "tool"},
	}
	got := e.String()
	if !strings.Contains(got, "[tool]") {
		t.Errorf("Entry.String() should include kind meta: %q", got)
	}
}

// ─── OpenFlag ───

func TestOpenFlagReadable(t *testing.T) {
	if !O_RDONLY.IsReadable() {
		t.Error("O_RDONLY should be readable")
	}
	if O_RDONLY.IsWritable() {
		t.Error("O_RDONLY should not be writable")
	}
	if !O_RDWR.IsReadable() {
		t.Error("O_RDWR should be readable")
	}
	if !O_RDWR.IsWritable() {
		t.Error("O_RDWR should be writable")
	}
	if !O_WRONLY.IsWritable() {
		t.Error("O_WRONLY should be writable")
	}
	if O_WRONLY.IsReadable() {
		t.Error("O_WRONLY should not be readable")
	}
}

func TestOpenFlagHas(t *testing.T) {
	f := O_WRONLY | O_CREATE | O_TRUNC
	if !f.Has(O_CREATE) {
		t.Error("combined flag should have O_CREATE")
	}
	if !f.Has(O_TRUNC) {
		t.Error("combined flag should have O_TRUNC")
	}
	if f.Has(O_APPEND) {
		t.Error("combined flag should not have O_APPEND")
	}
}

// ─── File ───

func TestNewFile(t *testing.T) {
	entry := &Entry{Name: "test.txt", Perm: PermRO}
	rc := io.NopCloser(strings.NewReader("hello"))
	f := NewFile("test.txt", entry, rc)

	if f.Name() != "test.txt" {
		t.Errorf("File.Name() = %q, want %q", f.Name(), "test.txt")
	}
	stat, err := f.Stat()
	if err != nil {
		t.Fatalf("File.Stat() error: %v", err)
	}
	if stat.Name != "test.txt" {
		t.Errorf("Stat().Name = %q, want %q", stat.Name, "test.txt")
	}

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("Read() = %q, want %q", string(data), "hello")
	}
}

func TestNewSeekableFile(t *testing.T) {
	entry := &Entry{Name: "seek.txt", Perm: PermRO}
	content := "hello world"
	sr := strings.NewReader(content)
	f := NewSeekableFile("seek.txt", entry, io.NopCloser(sr), sr)

	buf := make([]byte, 5)
	n, _ := f.Read(buf)
	if string(buf[:n]) != "hello" {
		t.Errorf("first read = %q, want %q", string(buf[:n]), "hello")
	}

	seeker := f.(io.Seeker)
	seeker.Seek(0, io.SeekStart)
	data, _ := io.ReadAll(f)
	if string(data) != content {
		t.Errorf("after seek, read = %q, want %q", string(data), content)
	}
}

func TestNewExecutableFile(t *testing.T) {
	entry := &Entry{Name: "run", Perm: PermRX}
	base := NewFile("run", entry, io.NopCloser(strings.NewReader("help text")))
	ef := NewExecutableFile(base, func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("executed: " + strings.Join(args, ","))), nil
	})

	if _, ok := ef.(ExecutableFile); !ok {
		t.Fatal("should implement ExecutableFile")
	}

	rc, err := ef.Exec(context.Background(), []string{"a", "b"}, nil)
	if err != nil {
		t.Fatalf("Exec error: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "executed: a,b" {
		t.Errorf("Exec output = %q, want %q", string(data), "executed: a,b")
	}
}

// ─── Errors ───

func TestErrorsSentinel(t *testing.T) {
	if ErrNotFound == nil {
		t.Error("ErrNotFound should not be nil")
	}
	if ErrNotFound.Error() != "shellfish: not found" {
		t.Errorf("ErrNotFound = %q", ErrNotFound.Error())
	}
}

// ─── SearchResult ───

func TestSearchResult(t *testing.T) {
	r := SearchResult{
		Entry:   Entry{Name: "doc.md", Path: "/docs/doc.md"},
		Snippet: "matching line",
		Score:   0.95,
	}
	if r.Score != 0.95 {
		t.Errorf("Score = %f, want 0.95", r.Score)
	}
	if r.Snippet != "matching line" {
		t.Errorf("Snippet = %q", r.Snippet)
	}
}

// ─── EventType ───

func TestEventTypeString(t *testing.T) {
	tests := []struct {
		event EventType
		want  string
	}{
		{EventCreate, "CREATE"},
		{EventWrite, "WRITE"},
		{EventRemove, "REMOVE"},
		{EventRename, "RENAME"},
		{EventMkdir, "MKDIR"},
		{EventAll, "CREATE|WRITE|REMOVE|RENAME|MKDIR"},
		{EventCreate | EventWrite, "CREATE|WRITE"},
		{EventType(0), "NONE"},
	}
	for _, tt := range tests {
		got := tt.event.String()
		if got != tt.want {
			t.Errorf("EventType(%d).String() = %q, want %q", tt.event, got, tt.want)
		}
	}
}

func TestEventTypeMatches(t *testing.T) {
	// Test that Matches returns true when the bit is set
	if !EventCreate.Matches(EventAll) {
		t.Error("EventCreate should match EventAll")
	}
	if !EventWrite.Matches(EventWrite) {
		t.Error("EventWrite should match EventWrite")
	}
	if !EventCreate.Matches(EventCreate | EventWrite) {
		t.Error("EventCreate should match EventCreate|EventWrite")
	}

	// Test that Matches returns false when the bit is not set
	if EventCreate.Matches(EventWrite) {
		t.Error("EventCreate should not match EventWrite")
	}
	if EventType(0).Matches(EventAll) {
		t.Error("NONE should not match EventAll")
	}
}
