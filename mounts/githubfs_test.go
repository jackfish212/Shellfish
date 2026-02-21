package mounts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackfish212/grasp/types"
)

func TestGitHubFS_Stat(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/owner/repo":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"repo","full_name":"owner/repo","description":"test repo","stargazers_count":100}`))
		case "/repos/owner/repo/contents":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"name":"README.md","path":"README.md","type":"file"}]`))
		case "/repos/owner/repo/issues/1":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"number":1,"title":"Test Issue","state":"open","body":"body","user":{"login":"user"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fs := NewGitHubFS(
		WithGitHubBaseURL(server.URL),
		WithGitHubToken("test-token"),
	)

	ctx := context.Background()

	tests := []struct {
		path    string
		wantDir bool
		wantErr bool
	}{
		{"/", true, false},
		{"/repos", true, false},
		{"/repos/owner", true, false},
		{"/repos/owner/repo", true, false},
		{"/repos/owner/repo/contents", true, false},
		{"/repos/owner/repo/issues", true, false},
		{"/repos/owner/repo/issues/1", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			entry, err := fs.Stat(ctx, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Stat(%s) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if err == nil && entry.IsDir != tt.wantDir {
				t.Errorf("Stat(%s) IsDir = %v, want %v", tt.path, entry.IsDir, tt.wantDir)
			}
		})
	}
}

func TestGitHubFS_List(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/users/testuser/repos":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"name":"repo1","full_name":"testuser/repo1","description":"repo 1","stargazers_count":10}]`))
		case "/repos/owner/repo/contents":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"name":"README.md","path":"README.md","type":"file"},{"name":"src","path":"src","type":"dir"}]`))
		case "/repos/owner/repo/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"number":1,"title":"Issue 1","state":"open","user":{"login":"user"}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fs := NewGitHubFS(
		WithGitHubBaseURL(server.URL),
		WithGitHubUser("testuser"),
	)

	ctx := context.Background()

	// Test root listing
	entries, err := fs.List(ctx, "/", types.ListOpts{})
	if err != nil {
		t.Fatalf("List(/) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "repos" {
		t.Errorf("List(/) = %v, want [repos]", entries)
	}

	// Test /repos listing
	entries, err = fs.List(ctx, "/repos", types.ListOpts{})
	if err != nil {
		t.Fatalf("List(/repos) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "repo1" {
		t.Errorf("List(/repos) = %v, want [repo1]", entries)
	}
}

func TestGitHubFS_Open(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Check for raw accept header for file content
		if r.Header.Get("Accept") == "application/vnd.github.raw+json" {
			if r.URL.Path == "/repos/owner/repo/contents/README.md" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("# Test README\n\nThis is a test."))
				return
			}
		}
		switch r.URL.Path {
		case "/repos/owner/repo/issues/1":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"number":1,"title":"Test Issue","state":"open","body":"Issue body","user":{"login":"user"},"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fs := NewGitHubFS(
		WithGitHubBaseURL(server.URL),
	)

	ctx := context.Background()

	// Test reading issue
	file, err := fs.Open(ctx, "/repos/owner/repo/issues/1")
	if err != nil {
		t.Fatalf("Open(issue) error = %v", err)
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("Read error = %v", err)
	}
	content := string(buf[:n])
	if content == "" {
		t.Error("Expected non-empty issue content")
	}
}

func TestGitHubFS_MountInfo(t *testing.T) {
	fs := NewGitHubFS()
	name, extra := fs.MountInfo()
	if name != "githubfs" {
		t.Errorf("MountInfo name = %s, want githubfs", name)
	}
	if extra != "github-api" {
		t.Errorf("MountInfo extra = %s, want github-api", extra)
	}
}

func TestGitHubFS_Cache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"name":"repo","full_name":"user/repo"}]`))
	}))
	defer server.Close()

	fs := NewGitHubFS(
		WithGitHubBaseURL(server.URL),
		WithGitHubCacheTTL(1*time.Minute),
	)

	ctx := context.Background()

	// First call
	_, err := fs.List(ctx, "/repos", types.ListOpts{})
	if err != nil {
		t.Fatalf("First List error = %v", err)
	}
	firstCount := callCount

	// Second call should hit cache
	_, err = fs.List(ctx, "/repos", types.ListOpts{})
	if err != nil {
		t.Fatalf("Second List error = %v", err)
	}

	if callCount != firstCount {
		t.Errorf("Cache not working: callCount = %d, expected %d", callCount, firstCount)
	}
}

func TestGitHubFS_Search(t *testing.T) {
	// Test that Search returns error for unsupported scopes
	fs := NewGitHubFS()
	ctx := context.Background()

	// Empty scope should fail
	_, err := fs.Search(ctx, "test", types.SearchOpts{})
	if err == nil {
		t.Error("Search with empty scope should fail")
	}

	// Non-issues scope should fail
	_, err = fs.Search(ctx, "test", types.SearchOpts{Scope: "/repos/owner/repo"})
	if err == nil {
		t.Error("Search with non-issues scope should fail")
	}
}
