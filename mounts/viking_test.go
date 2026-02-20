package mounts

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/jackfish212/shellfish/types"
)

// mockVikingClient implements VikingClient for testing without a real server.
type mockVikingClient struct {
	entries   map[string]*VikingEntry   // uri → entry
	children  map[string][]VikingEntry  // uri → children
	content   map[string]string         // uri → L2 content
	abstracts map[string]string         // uri → L0 abstract
	overviews map[string]string         // uri → L1 overview
	resources map[string]map[string]any // uri → add_resource result
	searchHits []VikingSearchHit
}

func newMockVikingClient() *mockVikingClient {
	return &mockVikingClient{
		entries:   make(map[string]*VikingEntry),
		children:  make(map[string][]VikingEntry),
		content:   make(map[string]string),
		abstracts: make(map[string]string),
		overviews: make(map[string]string),
		resources: make(map[string]map[string]any),
	}
}

func (m *mockVikingClient) Health(_ context.Context) (bool, error) { return true, nil }

func (m *mockVikingClient) Stat(_ context.Context, uri string) (*VikingEntry, error) {
	if e, ok := m.entries[uri]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("NOT_FOUND: %s", uri)
}

func (m *mockVikingClient) Ls(_ context.Context, uri string, _ bool) ([]VikingEntry, error) {
	if c, ok := m.children[uri]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("NOT_FOUND: %s", uri)
}

func (m *mockVikingClient) Mkdir(_ context.Context, uri string) error {
	m.entries[uri] = &VikingEntry{URI: uri, Name: baseName(strings.TrimPrefix(uri, "viking://")), IsDir: true}
	return nil
}

func (m *mockVikingClient) Remove(_ context.Context, uri string, _ bool) error {
	if _, ok := m.entries[uri]; !ok {
		return fmt.Errorf("NOT_FOUND: %s", uri)
	}
	delete(m.entries, uri)
	delete(m.children, uri)
	delete(m.content, uri)
	return nil
}

func (m *mockVikingClient) Move(_ context.Context, from, to string) error {
	e, ok := m.entries[from]
	if !ok {
		return fmt.Errorf("NOT_FOUND: %s", from)
	}
	delete(m.entries, from)
	e.URI = to
	m.entries[to] = e
	return nil
}

func (m *mockVikingClient) Read(_ context.Context, uri string) (string, error) {
	if c, ok := m.content[uri]; ok {
		return c, nil
	}
	return "", fmt.Errorf("NOT_FOUND: %s", uri)
}

func (m *mockVikingClient) Abstract(_ context.Context, uri string) (string, error) {
	if c, ok := m.abstracts[uri]; ok {
		return c, nil
	}
	return "", fmt.Errorf("NOT_FOUND: %s", uri)
}

func (m *mockVikingClient) Overview(_ context.Context, uri string) (string, error) {
	if c, ok := m.overviews[uri]; ok {
		return c, nil
	}
	return "", fmt.Errorf("NOT_FOUND: %s", uri)
}

func (m *mockVikingClient) AddResource(_ context.Context, path string, _ string) (map[string]any, error) {
	result := map[string]any{"root_uri": "viking://resources/" + baseName(path)}
	return result, nil
}

func (m *mockVikingClient) Find(_ context.Context, _ string, _ string, _ int) ([]VikingSearchHit, error) {
	return m.searchHits, nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestVikingProvider_StatRoot(t *testing.T) {
	p := NewVikingProvider(newMockVikingClient(), "")
	ctx := context.Background()

	entry, err := p.Stat(ctx, "")
	if err != nil {
		t.Fatalf("Stat root: %v", err)
	}
	if !entry.IsDir {
		t.Error("root should be a directory")
	}
}

func TestVikingProvider_StatEntry(t *testing.T) {
	mc := newMockVikingClient()
	mc.entries["viking://resources/project"] = &VikingEntry{
		URI: "viking://resources/project", Name: "project", IsDir: true,
		ContextType: "resource", Abstract: "A test project",
	}
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	entry, err := p.Stat(ctx, "resources/project")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if entry.Name != "project" {
		t.Errorf("name = %q, want %q", entry.Name, "project")
	}
	if !entry.IsDir {
		t.Error("should be a directory")
	}
	if entry.Meta["abstract"] != "A test project" {
		t.Errorf("abstract = %q, want %q", entry.Meta["abstract"], "A test project")
	}
}

func TestVikingProvider_StatNotFound(t *testing.T) {
	p := NewVikingProvider(newMockVikingClient(), "")
	ctx := context.Background()

	_, err := p.Stat(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should contain 'not found', got: %v", err)
	}
}

func TestVikingProvider_StatVirtualTierFiles(t *testing.T) {
	p := NewVikingProvider(newMockVikingClient(), "")
	ctx := context.Background()

	for _, name := range []string{".abstract", ".overview"} {
		entry, err := p.Stat(ctx, "resources/project/"+name)
		if err != nil {
			t.Fatalf("Stat %s: %v", name, err)
		}
		if entry.IsDir {
			t.Errorf("%s should not be a directory", name)
		}
		if entry.Meta["kind"] != "viking-tier" {
			t.Errorf("%s kind = %q, want %q", name, entry.Meta["kind"], "viking-tier")
		}
	}
}

func TestVikingProvider_List(t *testing.T) {
	mc := newMockVikingClient()
	mc.children["viking://resources"] = []VikingEntry{
		{URI: "viking://resources/docs", Name: "docs", IsDir: true},
		{URI: "viking://resources/readme.md", Name: "readme.md", IsDir: false},
	}
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	entries, err := p.List(ctx, "resources", types.ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Should have: docs, readme.md, .abstract, .overview
	if len(entries) != 4 {
		t.Fatalf("len(entries) = %d, want 4", len(entries))
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	for _, expected := range []string{"docs", "readme.md", ".abstract", ".overview"} {
		if !names[expected] {
			t.Errorf("missing entry %q", expected)
		}
	}
}

func TestVikingProvider_OpenRead(t *testing.T) {
	mc := newMockVikingClient()
	mc.content["viking://resources/readme.md"] = "# Hello World\n"
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	f, err := p.Open(ctx, "resources/readme.md")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(data) != "# Hello World\n" {
		t.Errorf("content = %q", string(data))
	}
}

func TestVikingProvider_OpenAbstract(t *testing.T) {
	mc := newMockVikingClient()
	mc.abstracts["viking://resources/project"] = "A project about testing."
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	f, err := p.Open(ctx, "resources/project/.abstract")
	if err != nil {
		t.Fatalf("Open .abstract: %v", err)
	}
	defer f.Close()

	data, _ := io.ReadAll(f)
	if string(data) != "A project about testing." {
		t.Errorf("abstract = %q", string(data))
	}
}

func TestVikingProvider_OpenOverview(t *testing.T) {
	mc := newMockVikingClient()
	mc.overviews["viking://resources/project"] = "Detailed overview of the project..."
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	f, err := p.Open(ctx, "resources/project/.overview")
	if err != nil {
		t.Fatalf("Open .overview: %v", err)
	}
	defer f.Close()

	data, _ := io.ReadAll(f)
	if string(data) != "Detailed overview of the project..." {
		t.Errorf("overview = %q", string(data))
	}
}

func TestVikingProvider_Write(t *testing.T) {
	mc := newMockVikingClient()
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	err := p.Write(ctx, "resources/new_doc", strings.NewReader("https://example.com/doc.md\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestVikingProvider_WriteEmptyErrors(t *testing.T) {
	p := NewVikingProvider(newMockVikingClient(), "")
	ctx := context.Background()

	err := p.Write(ctx, "resources/empty", strings.NewReader("  \n"))
	if err == nil {
		t.Fatal("expected error for empty resource path")
	}
}

func TestVikingProvider_Search(t *testing.T) {
	mc := newMockVikingClient()
	mc.searchHits = []VikingSearchHit{
		{URI: "viking://resources/auth.md", Abstract: "Authentication module", Score: 0.95},
		{URI: "viking://resources/login.md", Abstract: "Login flow docs", Score: 0.82},
	}
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	results, err := p.Search(ctx, "authentication", types.SearchOpts{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Score != 0.95 {
		t.Errorf("results[0].Score = %v, want 0.95", results[0].Score)
	}
	if results[0].Entry.Name != "auth.md" {
		t.Errorf("results[0].Entry.Name = %q, want %q", results[0].Entry.Name, "auth.md")
	}
}

func TestVikingProvider_Mkdir(t *testing.T) {
	mc := newMockVikingClient()
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	if err := p.Mkdir(ctx, "resources/new_dir", 0); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, ok := mc.entries["viking://resources/new_dir"]; !ok {
		t.Error("directory not created in mock")
	}
}

func TestVikingProvider_Remove(t *testing.T) {
	mc := newMockVikingClient()
	mc.entries["viking://resources/old"] = &VikingEntry{URI: "viking://resources/old", Name: "old", IsDir: true}
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	if err := p.Remove(ctx, "resources/old"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := mc.entries["viking://resources/old"]; ok {
		t.Error("entry should have been removed")
	}
}

func TestVikingProvider_Rename(t *testing.T) {
	mc := newMockVikingClient()
	mc.entries["viking://resources/a"] = &VikingEntry{URI: "viking://resources/a", Name: "a", IsDir: false}
	p := NewVikingProvider(mc, "")
	ctx := context.Background()

	if err := p.Rename(ctx, "resources/a", "resources/b"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, ok := mc.entries["viking://resources/a"]; ok {
		t.Error("old entry should not exist")
	}
	if _, ok := mc.entries["viking://resources/b"]; !ok {
		t.Error("new entry should exist")
	}
}

func TestVikingProvider_MountInfo(t *testing.T) {
	p := NewVikingProvider(newMockVikingClient(), "")
	name, extra := p.MountInfo()
	if name != "viking" {
		t.Errorf("name = %q, want %q", name, "viking")
	}
	if extra != "viking://" {
		t.Errorf("extra = %q, want %q", extra, "viking://")
	}
}

func TestVikingProvider_URIMapping(t *testing.T) {
	p := NewVikingProvider(newMockVikingClient(), "viking://")

	tests := []struct {
		path string
		uri  string
	}{
		{"", "viking://"},  // root URI preserves trailing slash
		{"resources", "viking://resources"},
		{"resources/project/docs", "viking://resources/project/docs"},
	}
	for _, tt := range tests {
		got := p.toURI(tt.path)
		if got != tt.uri {
			t.Errorf("toURI(%q) = %q, want %q", tt.path, got, tt.uri)
		}
	}
}
