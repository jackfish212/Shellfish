package httpfs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackfish212/grasp/types"
)

func TestNewHTTPFS(t *testing.T) {
	fs := NewHTTPFS()
	if fs == nil {
		t.Fatal("NewHTTPFS returned nil")
	}
	if fs.interval != 5*time.Minute {
		t.Errorf("default interval = %v, want 5m", fs.interval)
	}
	if fs.client == nil {
		t.Error("client is nil")
	}
}

func TestWithHTTPFSInterval(t *testing.T) {
	fs := NewHTTPFS(WithHTTPFSInterval(time.Minute))
	if fs.interval != time.Minute {
		t.Errorf("interval = %v, want 1m", fs.interval)
	}
}

func TestWithHTTPFSClient(t *testing.T) {
	customClient := &http.Client{Timeout: 10 * time.Second}
	fs := NewHTTPFS(WithHTTPFSClient(customClient))
	if fs.client != customClient {
		t.Error("client not set correctly")
	}
}

func TestAddSource(t *testing.T) {
	fs := NewHTTPFS()
	err := fs.Add("test", "https://example.com/feed", &RSSParser{})
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(Sources) = %d, want 1", len(sources))
	}
	if sources["test"] != "https://example.com/feed" {
		t.Errorf("sources[test] = %s", sources["test"])
	}

	// Duplicate should fail
	err = fs.Add("test", "https://other.com", &RSSParser{})
	if err == nil {
		t.Error("duplicate Add should fail")
	}
}

func TestRemoveSource(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("test", "https://example.com", &AutoParser{})

	err := fs.RemoveSource("test")
	if err != nil {
		t.Fatalf("RemoveSource failed: %v", err)
	}

	if len(fs.Sources()) != 0 {
		t.Error("source not removed")
	}

	err = fs.RemoveSource("nonexistent")
	if err == nil {
		t.Error("removing nonexistent source should fail")
	}
}

func TestStatRoot(t *testing.T) {
	fs := NewHTTPFS()
	entry, err := fs.Stat(context.Background(), "")
	if err != nil {
		t.Fatalf("Stat root failed: %v", err)
	}
	if !entry.IsDir {
		t.Error("root should be a directory")
	}
}

func TestStatSource(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("feed", "https://example.com/rss", &RSSParser{})

	entry, err := fs.Stat(context.Background(), "feed")
	if err != nil {
		t.Fatalf("Stat source failed: %v", err)
	}
	if !entry.IsDir {
		t.Error("source should be a directory")
	}
	if entry.Name != "feed" {
		t.Errorf("entry.Name = %s, want feed", entry.Name)
	}
}

func TestListRoot(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("a", "https://a.com", &AutoParser{})
	fs.Add("b", "https://b.com", &AutoParser{})

	entries, err := fs.List(context.Background(), "", types.ListOpts{})
	if err != nil {
		t.Fatalf("List root failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2", len(entries))
	}
	// Should be sorted
	if entries[0].Name > entries[1].Name {
		t.Error("entries not sorted")
	}
}

func TestFetchSource(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":1,"name":"item1"},{"id":2,"name":"item2"}]`))
	}))
	defer server.Close()

	fs := NewHTTPFS()
	parser := &JSONParser{NameField: "name", IDField: "id"}
	err := fs.Add("api", server.URL, parser)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Start polling (which fetches immediately)
	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	// Wait for fetch to complete
	time.Sleep(100 * time.Millisecond)

	// Check files were created
	entries, err := fs.List(context.Background(), "api", types.ListOpts{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2", len(entries))
		for _, e := range entries {
			t.Logf("  entry: %s", e.Name)
		}
	}
}

func TestRSSParser(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<item>
<title>First Post</title>
<link>https://example.com/1</link>
<description>Content of first post</description>
<pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
<guid>guid-1</guid>
</item>
<item>
<title>Second Post</title>
<link>https://example.com/2</link>
<description>Content of second post</description>
<pubDate>Tue, 02 Jan 2024 00:00:00 GMT</pubDate>
<guid>guid-2</guid>
</item>
</channel>
</rss>`

	parser := &RSSParser{}
	files, err := parser.Parse([]byte(rssXML))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if files[0].Name != "First Post" {
		t.Errorf("files[0].Name = %s", files[0].Name)
	}
	if !strings.Contains(files[0].Content, "Title: First Post") {
		t.Errorf("Content missing title: %s", files[0].Content)
	}
}

func TestAtomParser(t *testing.T) {
	atomXML := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<entry>
<title>Atom Entry</title>
<link href="https://example.com/atom/1" rel="alternate"/>
<summary>Atom summary</summary>
<updated>2024-01-01T00:00:00Z</updated>
<id>atom-id-1</id>
</entry>
</feed>`

	parser := &RSSParser{}
	files, err := parser.Parse([]byte(atomXML))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0].Name != "Atom Entry" {
		t.Errorf("files[0].Name = %s", files[0].Name)
	}
}

func TestJSONParser(t *testing.T) {
	jsonData := `[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]`

	parser := &JSONParser{NameField: "name", IDField: "id"}
	files, err := parser.Parse([]byte(jsonData))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if files[0].Name != "Alice" {
		t.Errorf("files[0].Name = %s, want Alice", files[0].Name)
	}
	if files[0].ID != "1" {
		t.Errorf("files[0].ID = %s, want 1", files[0].ID)
	}
}

func TestJSONParserNestedArray(t *testing.T) {
	jsonData := `{"data":{"items":[{"id":1,"title":"Item 1"},{"id":2,"title":"Item 2"}]}}`

	parser := &JSONParser{ArrayField: "data.items", NameField: "title", IDField: "id"}
	files, err := parser.Parse([]byte(jsonData))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if files[0].Name != "Item 1" {
		t.Errorf("files[0].Name = %s, want Item 1", files[0].Name)
	}
}

func TestRawParser(t *testing.T) {
	parser := &RawParser{Filename: "data"}
	files, err := parser.Parse([]byte("raw content"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0].Name != "data" {
		t.Errorf("files[0].Name = %s, want data", files[0].Name)
	}
	if files[0].Content != "raw content" {
		t.Errorf("files[0].Content = %s", files[0].Content)
	}
}

func TestAutoParser(t *testing.T) {
	parser := &AutoParser{}

	// Should detect JSON
	jsonFiles, err := parser.Parse([]byte(`[{"name":"test"}]`))
	if err != nil {
		t.Fatalf("Parse JSON failed: %v", err)
	}
	if len(jsonFiles) != 1 {
		t.Errorf("JSON len(files) = %d, want 1", len(jsonFiles))
	}

	// Should fallback to raw for plain text
	rawFiles, err := parser.Parse([]byte("plain text"))
	if err != nil {
		t.Fatalf("Parse raw failed: %v", err)
	}
	if len(rawFiles) != 1 {
		t.Errorf("Raw len(files) = %d, want 1", len(rawFiles))
	}
}

func TestMakeSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"This is a Test!", "this-is-a-test"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"  Trim  ", "trim"},
		{"Special @#$ Characters!", "special-characters"},
		{"123 Numbers 456", "123-numbers-456"},
		{"", "untitled"},
		{"   ", "untitled"},
	}

	for _, tt := range tests {
		result := makeSlug(tt.input)
		if result != tt.expected {
			t.Errorf("makeSlug(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestNormPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/", ""},
		{"/foo", "foo"},
		{"/foo/", "foo"},
		{"/foo/bar", "foo/bar"},
		{"/foo/bar/", "foo/bar"},
		{"foo", "foo"},
		{"foo/", "foo"},
	}

	for _, tt := range tests {
		result := normPath(tt.input)
		if result != tt.expected {
			t.Errorf("normPath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestWriteSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test content"))
	}))
	defer server.Close()

	fs := NewHTTPFS()
	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	err := fs.Write(context.Background(), "newsource", strings.NewReader(server.URL))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Wait for fetch
	time.Sleep(100 * time.Millisecond)

	sources := fs.Sources()
	if sources["newsource"] != server.URL {
		t.Errorf("sources[newsource] = %s", sources["newsource"])
	}
}

func TestLoadSchema(t *testing.T) {
	schema := `{
		"baseURL": "https://api.example.com",
		"defaults": {
			"headers": {"Authorization": "Bearer token"}
		},
		"sources": {
			"users": {"path": "/users", "parser": {"type": "json", "nameField": "name"}},
			"status": {"path": "/health", "parser": {"type": "raw"}}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadSchema([]byte(schema))
	if err != nil {
		t.Fatalf("LoadSchema failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 2 {
		t.Errorf("len(sources) = %d, want 2", len(sources))
	}
	if sources["users"] != "https://api.example.com/users" {
		t.Errorf("sources[users] = %s", sources["users"])
	}
}

func TestLoadOpenAPI(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/users": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"properties": {
												"id": {"type": "integer"},
												"name": {"type": "string"}
											}
										}
									}
								}
							}
						}
					}
				}
			},
			"/users/{id}": {
				"get": {
					"responses": {"200": {"description": "ok"}}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	// Should only have /users, not /users/{id}
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
	if _, ok := sources["users"]; !ok {
		t.Error("missing 'users' source")
	}
}

func TestOpenAndRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":1,"name":"test"}]`))
	}))
	defer server.Close()

	fs := NewHTTPFS()
	fs.Add("api", server.URL, &JSONParser{NameField: "name", IDField: "id"})

	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	time.Sleep(100 * time.Millisecond)

	// List to get the filename
	entries, err := fs.List(ctx, "api", types.ListOpts{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no files parsed")
	}

	// Open and read
	file, err := fs.Open(ctx, "api/"+entries[0].Name)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer file.Close()

	// Check stat
	entry, err := file.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if entry.Name != entries[0].Name {
		t.Errorf("entry.Name = %s, want %s", entry.Name, entries[0].Name)
	}
}

func TestRemoveViaRemoveMethod(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("test", "https://example.com", &AutoParser{})

	err := fs.Remove(context.Background(), "test")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if len(fs.Sources()) != 0 {
		t.Error("source not removed")
	}

	// Try to remove with path (should fail)
	err = fs.Remove(context.Background(), "test/file.txt")
	if err == nil {
		t.Error("removing file path should fail")
	}
}

func TestETagCaching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Header.Get("If-None-Match") == "test-etag" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", "test-etag")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":1,"name":"test"}]`))
	}))
	defer server.Close()

	fs := NewHTTPFS(WithHTTPFSInterval(50 * time.Millisecond))
	fs.Add("api", server.URL, &JSONParser{})

	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	// Wait for multiple fetch cycles
	time.Sleep(200 * time.Millisecond)

	// First fetch + at least one cached fetch
	if callCount < 2 {
		t.Errorf("callCount = %d, expected at least 2", callCount)
	}
}

func TestMountInfo(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("a", "https://a.com", &AutoParser{})
	fs.Add("b", "https://b.com", &AutoParser{})

	name, extra := fs.MountInfo()
	if name != "httpfs" {
		t.Errorf("name = %s, want httpfs", name)
	}
	if extra != "2 sources" {
		t.Errorf("extra = %s, want '2 sources'", extra)
	}
}

func TestMountInfoProvider(t *testing.T) {
	fs := NewHTTPFS()

	var _ types.MountInfoProvider = fs
}

// ─── Coverage tests for functions below 70% ───

func TestWithHTTPFSOnEvent(t *testing.T) {
	var events []struct {
		typ  types.EventType
		path string
	}
	fs := NewHTTPFS(WithHTTPFSOnEvent(func(et types.EventType, path string) {
		events = append(events, struct {
			typ  types.EventType
			path string
		}{et, path})
	}))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":1,"name":"item1"}]`))
	}))
	defer server.Close()

	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	fs.Add("api", server.URL, &JSONParser{NameField: "name", IDField: "id"})
	time.Sleep(100 * time.Millisecond)

	if len(events) == 0 {
		t.Error("expected event callback to be called, but got no events")
	}
	if events[0].typ != types.EventCreate {
		t.Errorf("event type = %v, want EventCreate", events[0].typ)
	}
}

func TestMkdir(t *testing.T) {
	fs := NewHTTPFS()
	err := fs.Mkdir(context.Background(), "testdir", types.PermRW)
	if err == nil {
		t.Error("Mkdir should return error")
	}
}

func TestRename(t *testing.T) {
	fs := NewHTTPFS()
	err := fs.Rename(context.Background(), "old", "new")
	if err == nil {
		t.Error("Rename should return error")
	}
}

func TestStatFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":1,"name":"test"}]`))
	}))
	defer server.Close()

	fs := NewHTTPFS()
	fs.Add("api", server.URL, &JSONParser{NameField: "name", IDField: "id"})

	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	time.Sleep(100 * time.Millisecond)

	// Stat a file (not just source directory)
	entries, err := fs.List(ctx, "api", types.ListOpts{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no entries")
	}

	entry, err := fs.Stat(ctx, "api/"+entries[0].Name)
	if err != nil {
		t.Fatalf("Stat file failed: %v", err)
	}
	if entry.Name != entries[0].Name {
		t.Errorf("entry.Name = %s, want %s", entry.Name, entries[0].Name)
	}
}

func TestStatNotFound(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("feed", "https://example.com", &AutoParser{})

	// Stat nonexistent source
	_, err := fs.Stat(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Stat nonexistent source should fail")
	}

	// Stat nonexistent file in existing source
	_, err = fs.Stat(context.Background(), "feed/nonexistent.txt")
	if err == nil {
		t.Error("Stat nonexistent file should fail")
	}
}

func TestFetchSourceWithHeaders(t *testing.T) {
	receivedHeader := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id":1,"name":"test"}]`))
	}))
	defer server.Close()

	fs := NewHTTPFS()
	fs.Add("api", server.URL, &JSONParser{}, WithSourceHeader("X-Custom-Header", "test-value"))

	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	time.Sleep(100 * time.Millisecond)

	if receivedHeader != "test-value" {
		t.Errorf("receivedHeader = %q, want 'test-value'", receivedHeader)
	}
}

func TestFetchSourceNonExistent(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("api", "https://example.com", &JSONParser{})

	// Manually remove source to test fetchSource with non-existent source
	fs.RemoveSource("api")

	ctx := context.Background()
	// This should not panic
	fs.Start(ctx)
	fs.Stop()
}

func TestFetchSourceHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fs := NewHTTPFS()
	fs.Add("api", server.URL, &JSONParser{})

	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	time.Sleep(100 * time.Millisecond)

	// Should not have any files
	entries, err := fs.List(ctx, "api", types.ListOpts{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files on HTTP error, got %d", len(entries))
	}
}

func TestFetchSourceParserError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid rss"))
	}))
	defer server.Close()

	fs := NewHTTPFS()
	fs.Add("api", server.URL, &RSSParser{}) // RSSParser will fail on non-RSS content

	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	time.Sleep(100 * time.Millisecond)

	// AutoParser falls back to raw, but RSSParser returns error
	entries, err := fs.List(ctx, "api", types.ListOpts{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files on parser error, got %d", len(entries))
	}
}

func TestWriteUpdateExisting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("content"))
	}))
	defer server.Close()

	fs := NewHTTPFS()
	ctx := context.Background()
	fs.Start(ctx)
	defer fs.Stop()

	// Add source first
	fs.Add("existing", server.URL, &RawParser{})
	time.Sleep(50 * time.Millisecond)

	// Update via Write
	err := fs.Write(context.Background(), "existing", strings.NewReader(server.URL+"/updated"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	sources := fs.Sources()
	if sources["existing"] != server.URL+"/updated" {
		t.Errorf("URL not updated: %s", sources["existing"])
	}
}

func TestWriteErrors(t *testing.T) {
	fs := NewHTTPFS()

	// Write with path separator should fail
	err := fs.Write(context.Background(), "foo/bar", strings.NewReader("https://example.com"))
	if err == nil {
		t.Error("Write with path separator should fail")
	}

	// Write with empty path should fail
	err = fs.Write(context.Background(), "", strings.NewReader("https://example.com"))
	if err == nil {
		t.Error("Write with empty path should fail")
	}

	// Write with empty URL should fail
	err = fs.Write(context.Background(), "test", strings.NewReader(""))
	if err == nil {
		t.Error("Write with empty URL should fail")
	}
}

func TestOpenErrors(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("feed", "https://example.com", &AutoParser{})

	// Open root path (should fail - is dir)
	_, err := fs.Open(context.Background(), "")
	if err == nil {
		t.Error("Open root should fail")
	}

	// Open source dir (should fail - is dir)
	_, err = fs.Open(context.Background(), "feed")
	if err == nil {
		t.Error("Open source dir should fail")
	}

	// Open nonexistent source
	_, err = fs.Open(context.Background(), "nonexistent/file.txt")
	if err == nil {
		t.Error("Open nonexistent source should fail")
	}

	// Open nonexistent file in existing source
	_, err = fs.Open(context.Background(), "feed/nonexistent.txt")
	if err == nil {
		t.Error("Open nonexistent file should fail")
	}
}

func TestListErrors(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("feed", "https://example.com", &AutoParser{})

	// List nonexistent source
	_, err := fs.List(context.Background(), "nonexistent", types.ListOpts{})
	if err == nil {
		t.Error("List nonexistent source should fail")
	}

	// List nested path (not a directory)
	_, err = fs.List(context.Background(), "feed/subpath", types.ListOpts{})
	if err == nil {
		t.Error("List nested path should fail")
	}
}

func TestRemoveErrors(t *testing.T) {
	fs := NewHTTPFS()
	fs.Add("feed", "https://example.com", &AutoParser{})

	// Remove with path separator
	err := fs.Remove(context.Background(), "feed/file.txt")
	if err == nil {
		t.Error("Remove with path separator should fail")
	}

	// Remove empty path
	err = fs.Remove(context.Background(), "")
	if err == nil {
		t.Error("Remove empty path should fail")
	}
}

func TestRawParserWithCustomFilename(t *testing.T) {
	parser := &RawParser{Filename: "custom-name"}
	files, err := parser.Parse([]byte("test"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if files[0].Name != "custom-name" {
		t.Errorf("Name = %s, want custom-name", files[0].Name)
	}
}

func TestRawParserDefaultFilename(t *testing.T) {
	parser := &RawParser{}
	files, err := parser.Parse([]byte("test"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if files[0].Name != "content" {
		t.Errorf("Name = %s, want content", files[0].Name)
	}
}

func TestAutoParserFallback(t *testing.T) {
	parser := &AutoParser{}

	// Invalid RSS should fallback to raw
	files, err := parser.Parse([]byte("not rss at all"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("len(files) = %d, want 1", len(files))
	}
	if files[0].Content != "not rss at all" {
		t.Errorf("Content = %s, want 'not rss at all'", files[0].Content)
	}
}

// ─── Schema coverage tests ───

func TestLoadSchemaWithRSSParser(t *testing.T) {
	schema := `{
		"sources": {
			"feed": {"url": "https://example.com/rss", "parser": {"type": "rss"}}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadSchema([]byte(schema))
	if err != nil {
		t.Fatalf("LoadSchema failed: %v", err)
	}

	sources := fs.Sources()
	if sources["feed"] != "https://example.com/rss" {
		t.Errorf("sources[feed] = %s", sources["feed"])
	}
}

func TestLoadSchemaWithAutoParser(t *testing.T) {
	schema := `{
		"sources": {
			"auto": {"url": "https://example.com/data", "parser": {"type": "auto"}}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadSchema([]byte(schema))
	if err != nil {
		t.Fatalf("LoadSchema failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadSchemaWithRawParser(t *testing.T) {
	schema := `{
		"sources": {
			"raw": {"url": "https://example.com/raw", "parser": {"type": "raw", "filename": "data"}}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadSchema([]byte(schema))
	if err != nil {
		t.Fatalf("LoadSchema failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadSchemaWithSourceHeaders(t *testing.T) {
	schema := `{
		"defaults": {
			"headers": {"X-Default": "default-value"}
		},
		"sources": {
			"test": {
				"url": "https://example.com/test",
				"headers": {"X-Custom": "custom-value"},
				"parser": {"type": "json"}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadSchema([]byte(schema))
	if err != nil {
		t.Fatalf("LoadSchema failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadSchemaErrors(t *testing.T) {
	fs := NewHTTPFS()

	// Invalid JSON
	err := fs.LoadSchema([]byte("not json"))
	if err == nil {
		t.Error("LoadSchema with invalid JSON should fail")
	}

	// Missing url and path
	schema := `{"sources": {"test": {"parser": {"type": "auto"}}}}`
	err = fs.LoadSchema([]byte(schema))
	if err == nil {
		t.Error("LoadSchema without url/path should fail")
	}

	// Duplicate source via schema
	schema = `{"sources": {"dup": {"url": "https://a.com"}, "dup2": {"url": "https://b.com"}}}`
	fs.LoadSchema([]byte(schema))
	schema = `{"sources": {"dup": {"url": "https://c.com"}}}`
	err = fs.LoadSchema([]byte(schema))
	if err == nil {
		t.Error("LoadSchema duplicate source should fail")
	}
}

func TestLoadOpenAPIFromURL(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/users": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"properties": {
												"id": {"type": "integer"},
												"name": {"type": "string"}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(spec))
	}))
	defer server.Close()

	fs := NewHTTPFS()
	err := fs.LoadOpenAPIFromURL(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("LoadOpenAPIFromURL failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadOpenAPIFromURLErrors(t *testing.T) {
	fs := NewHTTPFS()

	// Invalid URL
	err := fs.LoadOpenAPIFromURL(context.Background(), "://invalid")
	if err == nil {
		t.Error("LoadOpenAPIFromURL with invalid URL should fail")
	}

	// HTTP error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	err = fs.LoadOpenAPIFromURL(context.Background(), server.URL)
	if err == nil {
		t.Error("LoadOpenAPIFromURL with HTTP 404 should fail")
	}

	// Invalid JSON response
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	err = fs.LoadOpenAPIFromURL(context.Background(), server.URL)
	if err == nil {
		t.Error("LoadOpenAPIFromURL with invalid JSON should fail")
	}
}

func TestLoadOpenAPIWithRef(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"components": {
			"schemas": {
				"User": {
					"type": "object",
					"properties": {
						"id": {"type": "integer"},
						"name": {"type": "string"}
					}
				}
			}
		},
		"paths": {
			"/users": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"$ref": "#/components/schemas/User"
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadOpenAPIWithRefResolveErrors(t *testing.T) {
	// Test with invalid ref path (should still work, just won't resolve)
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/users": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"$ref": "#/nonexistent/path"
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	// Should still create the source even if ref doesn't resolve
	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadOpenAPIWithObjectResponse(t *testing.T) {
	// Test with object response (not array) - should use RawParser
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/status": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "object",
										"properties": {
											"status": {"type": "string"}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadOpenAPIWithNoResponses(t *testing.T) {
	// Test operation with no responses defined
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/users": {
				"get": {}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	// Should still create source with AutoParser
	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadOpenAPIWithNo200Response(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/users": {
				"get": {
					"responses": {
						"404": {"description": "not found"}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	// Should still create source with AutoParser
	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestLoadOpenAPIWithOtherContentType(t *testing.T) {
	// Test with non-JSON content type
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/data": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"text/plain": {
									"schema": {"type": "string"}
								}
							}
						}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestOpenAPIPathToNameEdgeCases(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/", "root"},
		{"", "root"},
		{"/{id}", "root"},
		{"/api/v1/users", "api-v1-users"},
	}

	for _, tt := range tests {
		result := openAPIPathToName(tt.path)
		if result != tt.expected {
			t.Errorf("openAPIPathToName(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestLoadOpenAPIWithExtraSourceOptions(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/users": {
				"get": {
					"responses": {
						"200": {"description": "ok"}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec), WithSourceHeader("X-Custom", "value"))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestResolveOpenAPISchemaWithExternalRef(t *testing.T) {
	// External refs (not starting with #/) should not be resolved
	schema := &openAPISchema{
		Ref: "https://example.com/schema.json",
	}
	result := resolveOpenAPISchema(map[string]any{}, schema)
	if result != schema {
		t.Error("External ref should return original schema")
	}
}

func TestResolveOpenAPISchemaWithNil(t *testing.T) {
	result := resolveOpenAPISchema(map[string]any{}, nil)
	if result != nil {
		t.Error("Nil schema should return nil")
	}
}

func TestResolveOpenAPISchemaNoRef(t *testing.T) {
	schema := &openAPISchema{Type: "string"}
	result := resolveOpenAPISchema(map[string]any{}, schema)
	if result != schema {
		t.Error("Schema without $ref should return itself")
	}
}

func TestInferParserFromOpenAPIWithIDField(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/items": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"properties": {
												"_id": {"type": "string"},
												"title": {"type": "string"}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestInferParserFromOpenAPIWithUUID(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/items": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"properties": {
												"uuid": {"type": "string"},
												"label": {"type": "string"}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestInferParserFromOpenAPIWithKey(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/items": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"properties": {
												"key": {"type": "string"},
												"slug": {"type": "string"}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}

func TestInferParserFromOpenAPIWithUsername(t *testing.T) {
	spec := `{
		"openapi": "3.0.0",
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/users": {
				"get": {
					"responses": {
						"200": {
							"content": {
								"application/json": {
									"schema": {
										"type": "array",
										"items": {
											"properties": {
												"id": {"type": "integer"},
												"username": {"type": "string"}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	fs := NewHTTPFS()
	err := fs.LoadOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("LoadOpenAPI failed: %v", err)
	}

	sources := fs.Sources()
	if len(sources) != 1 {
		t.Errorf("len(sources) = %d, want 1", len(sources))
	}
}
