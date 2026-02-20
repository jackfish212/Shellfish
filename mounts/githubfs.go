// Package mounts provides built-in Mount implementations for shellfish.
package mounts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jackfish212/shellfish/types"
)

// Compile-time interface checks
var (
	_ types.Provider   = (*GitHubFS)(nil)
	_ types.Readable   = (*GitHubFS)(nil)
	_ types.Searchable = (*GitHubFS)(nil)
)

// GitHubFS mounts GitHub API as a virtual filesystem.
//
// Filesystem layout:
//
//	/repos                           - list user's repositories (or orgs if configured)
//	/repos/{owner}/{repo}            - repository info
//	/repos/{owner}/{repo}/contents/... - repository files (read-only)
//	/repos/{owner}/{repo}/issues     - list issues
//	/repos/{owner}/{repo}/issues/{N} - read issue N
//
// Example:
//
//	ls /repos                           -> list repositories
//	cat /repos/golang/go/README.md      -> read file from go repo
//	cat /repos/golang/go/issues/123     -> read issue #123
//	search "bug" --scope /repos/owner/repo/issues
type GitHubFS struct {
	client    *http.Client
	token     string
	baseURL   string
	user      string // GitHub username/org for /repos listing
	perm      types.Perm
	cache     map[string]*cacheEntry
	cacheMu   sync.RWMutex
	cacheTTL  time.Duration
}

type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

// GitHubFSOption configures the GitHubFS.
type GitHubFSOption func(*GitHubFS)

// WithGitHubToken sets the GitHub personal access token.
func WithGitHubToken(token string) GitHubFSOption {
	return func(fs *GitHubFS) { fs.token = token }
}

// WithGitHubUser sets the default user/org for /repos listing.
func WithGitHubUser(user string) GitHubFSOption {
	return func(fs *GitHubFS) { fs.user = user }
}

// WithGitHubBaseURL sets a custom API base URL (e.g., for GitHub Enterprise).
func WithGitHubBaseURL(url string) GitHubFSOption {
	return func(fs *GitHubFS) { fs.baseURL = url }
}

// WithGitHubCacheTTL sets the cache TTL (default 5 minutes).
func WithGitHubCacheTTL(ttl time.Duration) GitHubFSOption {
	return func(fs *GitHubFS) { fs.cacheTTL = ttl }
}

// NewGitHubFS creates a new GitHub filesystem provider.
func NewGitHubFS(opts ...GitHubFSOption) *GitHubFS {
	fs := &GitHubFS{
		client:   &http.Client{Timeout: 30 * time.Second},
		baseURL:  "https://api.github.com",
		perm:     types.PermRO,
		cache:    make(map[string]*cacheEntry),
		cacheTTL: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(fs)
	}
	return fs
}

// Stat returns information about a path.
func (fs *GitHubFS) Stat(ctx context.Context, path string) (*types.Entry, error) {
	path = normPath(path)

	// Root
	if path == "" {
		return &types.Entry{Name: "/", Path: "/", IsDir: true, Perm: types.PermRX}, nil
	}

	parts := strings.Split(path, "/")

	// /repos
	if parts[0] == "repos" {
		return fs.statRepos(ctx, parts)
	}

	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
}

func (fs *GitHubFS) statRepos(ctx context.Context, parts []string) (*types.Entry, error) {
	switch len(parts) {
	case 1:
		// /repos
		return &types.Entry{Name: "repos", Path: "repos", IsDir: true, Perm: types.PermRX}, nil

	case 2:
		// /repos/{owner}
		return &types.Entry{Name: parts[1], Path: "repos/" + parts[1], IsDir: true, Perm: types.PermRX}, nil

	case 3:
		// /repos/{owner}/{repo}
		repo, err := fs.getRepo(ctx, parts[1], parts[2])
		if err != nil {
			return nil, err
		}
		return &types.Entry{
			Name:  parts[2],
			Path:  "repos/" + parts[1] + "/" + parts[2],
			IsDir: true,
			Perm:  types.PermRX,
			Meta:  map[string]string{"description": repo.Description, "stars": fmt.Sprintf("%d", repo.StargazersCount)},
		}, nil

	case 4:
		// /repos/{owner}/{repo}/contents, /repos/{owner}/{repo}/issues
		return &types.Entry{Name: parts[3], Path: strings.Join(parts, "/"), IsDir: true, Perm: types.PermRX}, nil

	case 5:
		// /repos/{owner}/{repo}/contents/{path} or /repos/{owner}/{repo}/issues/{N}
		if parts[3] == "issues" {
			issue, err := fs.getIssue(ctx, parts[1], parts[2], parts[4])
			if err != nil {
				return nil, err
			}
			return &types.Entry{
				Name:  parts[4],
				Path:  strings.Join(parts, "/"),
				IsDir: false,
				Perm:  types.PermRO,
				Meta:  map[string]string{"title": issue.Title, "state": issue.State},
			}, nil
		}
		if parts[3] == "contents" {
			content, err := fs.getContentInfo(ctx, parts[1], parts[2], parts[4])
			if err != nil {
				return nil, err
			}
			return &types.Entry{
				Name:  parts[4],
				Path:  strings.Join(parts, "/"),
				IsDir: content.Type == "dir",
				Perm:  types.PermRO,
			}, nil
		}

	default:
		// /repos/{owner}/{repo}/contents/{path...}
		if parts[3] == "contents" {
			contentPath := strings.Join(parts[4:], "/")
			content, err := fs.getContentInfo(ctx, parts[1], parts[2], contentPath)
			if err != nil {
				return nil, err
			}
			return &types.Entry{
				Name:  parts[len(parts)-1],
				Path:  strings.Join(parts, "/"),
				IsDir: content.Type == "dir",
				Perm:  types.PermRO,
			}, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, strings.Join(parts, "/"))
}

// List lists entries in a directory.
func (fs *GitHubFS) List(ctx context.Context, path string, opts types.ListOpts) ([]types.Entry, error) {
	path = normPath(path)
	parts := strings.Split(path, "/")

	if path == "" {
		return []types.Entry{
			{Name: "repos", Path: "repos", IsDir: true, Perm: types.PermRX},
		}, nil
	}

	if parts[0] == "repos" {
		return fs.listRepos(ctx, parts)
	}

	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
}

func (fs *GitHubFS) listRepos(ctx context.Context, parts []string) ([]types.Entry, error) {
	switch len(parts) {
	case 1:
		// /repos - list repositories
		return fs.listRepositories(ctx)

	case 2:
		// /repos/{owner} - list owner's repos
		return fs.listOwnerRepos(ctx, parts[1])

	case 3:
		// /repos/{owner}/{repo} - list repo subdirs
		return []types.Entry{
			{Name: "contents", Path: "repos/" + parts[1] + "/" + parts[2] + "/contents", IsDir: true, Perm: types.PermRX},
			{Name: "issues", Path: "repos/" + parts[1] + "/" + parts[2] + "/issues", IsDir: true, Perm: types.PermRX},
		}, nil

	case 4:
		// /repos/{owner}/{repo}/contents or /repos/{owner}/{repo}/issues
		switch parts[3] {
		case "contents":
			return fs.listContents(ctx, parts[1], parts[2], "")
		case "issues":
			return fs.listIssues(ctx, parts[1], parts[2])
		}

	default:
		// /repos/{owner}/{repo}/contents/{path...}
		if parts[3] == "contents" {
			contentPath := strings.Join(parts[4:], "/")
			return fs.listContents(ctx, parts[1], parts[2], contentPath)
		}
	}

	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, strings.Join(parts, "/"))
}

// Open opens a file for reading.
func (fs *GitHubFS) Open(ctx context.Context, path string) (types.File, error) {
	path = normPath(path)
	parts := strings.Split(path, "/")

	if len(parts) < 4 {
		return nil, fmt.Errorf("%w: %s is a directory", types.ErrIsDir, path)
	}

	if parts[0] != "repos" {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}

	var content []byte
	var entry *types.Entry

	switch parts[3] {
	case "issues":
		if len(parts) < 5 {
			return nil, fmt.Errorf("%w: %s is a directory", types.ErrIsDir, path)
		}
		issue, err := fs.getIssue(ctx, parts[1], parts[2], parts[4])
		if err != nil {
			return nil, err
		}
		content = []byte(fs.formatIssue(issue))
		entry = &types.Entry{
			Name:  parts[4],
			Path:  path,
			IsDir: false,
			Perm:  types.PermRO,
			Meta:  map[string]string{"title": issue.Title},
		}

	case "contents":
		if len(parts) < 5 {
			return nil, fmt.Errorf("%w: %s is a directory", types.ErrIsDir, path)
		}
		contentPath := strings.Join(parts[4:], "/")
		data, err := fs.getFileContent(ctx, parts[1], parts[2], contentPath)
		if err != nil {
			return nil, err
		}
		content = data
		entry = &types.Entry{
			Name:  parts[len(parts)-1],
			Path:  path,
			IsDir: false,
			Perm:  types.PermRO,
		}

	default:
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}

	return types.NewFile(path, entry, io.NopCloser(bytes.NewReader(content))), nil
}

// Search searches for issues matching a query.
func (fs *GitHubFS) Search(ctx context.Context, query string, opts types.SearchOpts) ([]types.SearchResult, error) {
	// Parse scope to determine search type
	scope := normPath(opts.Scope)
	parts := strings.Split(scope, "/")

	maxResults := opts.MaxResults
	if maxResults == 0 {
		maxResults = 20
	}

	// Currently only support searching issues
	if len(parts) >= 4 && parts[0] == "repos" && parts[3] == "issues" {
		owner, repo := parts[1], parts[2]
		return fs.searchIssues(ctx, owner, repo, query, maxResults)
	}

	return nil, fmt.Errorf("search not supported for path: %s", scope)
}

// --- GitHub API types ---

type githubRepo struct {
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	StargazersCount int  `json:"stargazers_count"`
	Private       bool   `json:"private"`
}

type githubContent struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "file" or "dir"
}

type githubIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	Body      string    `json:"body"`
	User      struct{ Login string } `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Labels    []struct{ Name string } `json:"labels"`
}

type githubSearchResult struct {
	TotalCount int            `json:"total_count"`
	Items      []githubIssue  `json:"items"`
}

// --- API methods ---

func (fs *GitHubFS) listRepositories(ctx context.Context) ([]types.Entry, error) {
	user := fs.user
	if user == "" {
		// List authenticated user's repos
		var repos []githubRepo
		if err := fs.apiGet(ctx, "/user/repos?per_page=100", &repos); err != nil {
			return nil, err
		}
		return fs.reposToEntries(repos), nil
	}

	// List specific user's repos
	var repos []githubRepo
	if err := fs.apiGet(ctx, "/users/"+user+"/repos?per_page=100", &repos); err != nil {
		return nil, err
	}
	return fs.reposToEntries(repos), nil
}

func (fs *GitHubFS) listOwnerRepos(ctx context.Context, owner string) ([]types.Entry, error) {
	var repos []githubRepo
	if err := fs.apiGet(ctx, "/users/"+owner+"/repos?per_page=100", &repos); err != nil {
		return nil, err
	}
	return fs.reposToEntries(repos), nil
}

func (fs *GitHubFS) getRepo(ctx context.Context, owner, repo string) (*githubRepo, error) {
	var r githubRepo
	if err := fs.apiGet(ctx, "/repos/"+owner+"/"+repo, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (fs *GitHubFS) listContents(ctx context.Context, owner, repo, path string) ([]types.Entry, error) {
	var contents []githubContent
	apiPath := "/repos/" + owner + "/" + repo + "/contents"
	if path != "" {
		apiPath += "/" + path
	}
	if err := fs.apiGet(ctx, apiPath, &contents); err != nil {
		return nil, err
	}

	var entries []types.Entry
	entryPath := "repos/" + owner + "/" + repo + "/contents"
	if path != "" {
		entryPath += "/" + path
	}
	for _, c := range contents {
		entries = append(entries, types.Entry{
			Name:  c.Name,
			Path:  entryPath + "/" + c.Name,
			IsDir: c.Type == "dir",
			Perm:  types.PermRO,
		})
	}
	return entries, nil
}

func (fs *GitHubFS) getContentInfo(ctx context.Context, owner, repo, path string) (*githubContent, error) {
	var contents []githubContent
	apiPath := "/repos/" + owner + "/" + repo + "/contents/" + path
	if err := fs.apiGet(ctx, apiPath, &contents); err != nil {
		// Try as file
		var c githubContent
		if err2 := fs.apiGet(ctx, apiPath, &c); err2 != nil {
			return nil, err
		}
		return &c, nil
	}
	// It's a directory
	return &githubContent{Name: baseName(path), Type: "dir"}, nil
}

func (fs *GitHubFS) getFileContent(ctx context.Context, owner, repo, path string) ([]byte, error) {
	apiPath := "/repos/" + owner + "/" + repo + "/contents/" + path

	// Use raw accept header to get raw content
	req, err := http.NewRequestWithContext(ctx, "GET", fs.baseURL+apiPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	if fs.token != "" {
		req.Header.Set("Authorization", "Bearer "+fs.token)
	}

	resp, err := fs.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api error: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func (fs *GitHubFS) listIssues(ctx context.Context, owner, repo string) ([]types.Entry, error) {
	var issues []githubIssue
	if err := fs.apiGet(ctx, "/repos/"+owner+"/"+repo+"/issues?state=all&per_page=100", &issues); err != nil {
		return nil, err
	}

	var entries []types.Entry
	for _, issue := range issues {
		entries = append(entries, types.Entry{
			Name:  fmt.Sprintf("%d", issue.Number),
			Path:  "repos/" + owner + "/" + repo + "/issues/" + fmt.Sprintf("%d", issue.Number),
			IsDir: false,
			Perm:  types.PermRO,
			Meta:  map[string]string{"title": issue.Title, "state": issue.State},
		})
	}
	return entries, nil
}

func (fs *GitHubFS) getIssue(ctx context.Context, owner, repo, number string) (*githubIssue, error) {
	var issue githubIssue
	if err := fs.apiGet(ctx, "/repos/"+owner+"/"+repo+"/issues/"+number, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

func (fs *GitHubFS) searchIssues(ctx context.Context, owner, repo, query string, maxResults int) ([]types.SearchResult, error) {
	q := fmt.Sprintf("%s repo:%s/%s is:issue", url.QueryEscape(query), owner, repo)
	var result githubSearchResult
	if err := fs.apiGet(ctx, "/search/issues?q="+q+"&per_page="+fmt.Sprintf("%d", maxResults), &result); err != nil {
		return nil, err
	}

	var results []types.SearchResult
	for _, item := range result.Items {
		results = append(results, types.SearchResult{
			Entry: types.Entry{
				Name:  fmt.Sprintf("%d", item.Number),
				Path:  "repos/" + owner + "/" + repo + "/issues/" + fmt.Sprintf("%d", item.Number),
				IsDir: false,
				Perm:  types.PermRO,
				Meta:  map[string]string{"title": item.Title},
			},
			Snippet: truncateString(item.Body, 200),
			Score:   1.0,
		})
	}
	return results, nil
}

// --- Helpers ---

func (fs *GitHubFS) apiGet(ctx context.Context, path string, v interface{}) error {
	// Check cache
	fs.cacheMu.RLock()
	if entry, ok := fs.cache[path]; ok && time.Now().Before(entry.expiresAt) {
		fs.cacheMu.RUnlock()
		return json.Unmarshal(entry.data, v)
	}
	fs.cacheMu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", fs.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if fs.token != "" {
		req.Header.Set("Authorization", "Bearer "+fs.token)
	}
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := fs.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api error: %s - %s", resp.Status, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Cache the result
	fs.cacheMu.Lock()
	fs.cache[path] = &cacheEntry{data: data, expiresAt: time.Now().Add(fs.cacheTTL)}
	fs.cacheMu.Unlock()

	return json.Unmarshal(data, v)
}

func (fs *GitHubFS) reposToEntries(repos []githubRepo) []types.Entry {
	var entries []types.Entry
	for _, r := range repos {
		ownerRepo := strings.SplitN(r.FullName, "/", 2)
		if len(ownerRepo) != 2 {
			continue
		}
		entries = append(entries, types.Entry{
			Name:  ownerRepo[1],
			Path:  "repos/" + r.FullName,
			IsDir: true,
			Perm:  types.PermRX,
			Meta:  map[string]string{"description": r.Description, "stars": fmt.Sprintf("%d", r.StargazersCount)},
		})
	}
	return entries
}

func (fs *GitHubFS) formatIssue(issue *githubIssue) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "Issue #%d: %s\n", issue.Number, issue.Title)
	fmt.Fprintf(&buf, "State: %s\n", issue.State)
	fmt.Fprintf(&buf, "Author: %s\n", issue.User.Login)
	fmt.Fprintf(&buf, "Created: %s\n", issue.CreatedAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(&buf, "Updated: %s\n", issue.UpdatedAt.Format("2006-01-02 15:04"))

	if len(issue.Labels) > 0 {
		labels := make([]string, len(issue.Labels))
		for i, l := range issue.Labels {
			labels[i] = l.Name
		}
		fmt.Fprintf(&buf, "Labels: %s\n", strings.Join(labels, ", "))
	}

	fmt.Fprintf(&buf, "\n---\n\n%s\n", issue.Body)
	return buf.String()
}

func (fs *GitHubFS) MountInfo() (string, string) {
	return "githubfs", "github-api"
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
