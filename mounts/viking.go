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
	"time"

	"github.com/jackfish212/grasp/types"
)

// VikingClient abstracts the OpenViking HTTP API.
type VikingClient interface {
	Health(ctx context.Context) (bool, error)

	// Filesystem
	Ls(ctx context.Context, uri string, recursive bool) ([]VikingEntry, error)
	Stat(ctx context.Context, uri string) (*VikingEntry, error)
	Mkdir(ctx context.Context, uri string) error
	Remove(ctx context.Context, uri string, recursive bool) error
	Move(ctx context.Context, fromURI, toURI string) error

	// Content
	Read(ctx context.Context, uri string) (string, error)
	Abstract(ctx context.Context, uri string) (string, error)
	Overview(ctx context.Context, uri string) (string, error)

	// Resources
	AddResource(ctx context.Context, path string, target string) (map[string]any, error)

	// Search
	Find(ctx context.Context, query string, targetURI string, limit int) ([]VikingSearchHit, error)
}

// VikingEntry represents a filesystem entry returned by OpenViking.
type VikingEntry struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	IsDir       bool   `json:"is_dir"`
	Abstract    string `json:"abstract"`
	ContextType string `json:"context_type"`
	Size        int64  `json:"size"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// VikingSearchHit represents a search result from OpenViking.
type VikingSearchHit struct {
	URI      string  `json:"uri"`
	Abstract string  `json:"abstract"`
	Score    float64 `json:"score"`
	Content  string  `json:"content"`
}

// vikingHTTPClient is the default HTTP-based VikingClient.
type vikingHTTPClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewVikingClient creates a VikingClient that talks to an OpenViking server.
func NewVikingClient(baseURL string, apiKey string) VikingClient {
	return &vikingHTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *vikingHTTPClient) doRequest(ctx context.Context, method, path string, query url.Values, body any) (json.RawMessage, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("viking: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("viking: create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("viking: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("viking: read response: %w", err)
	}

	var envelope struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("viking: HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return respBody, nil
	}

	if envelope.Status == "error" {
		return nil, fmt.Errorf("viking: %s: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("viking: HTTP %d: %s", resp.StatusCode, envelope.Error.Message)
	}

	return envelope.Result, nil
}

func (c *vikingHTTPClient) Health(ctx context.Context) (bool, error) {
	raw, err := c.doRequest(ctx, "GET", "/health", nil, nil)
	if err != nil {
		return false, err
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return false, nil
	}
	return resp.Status == "ok", nil
}

func (c *vikingHTTPClient) Ls(ctx context.Context, uri string, recursive bool) ([]VikingEntry, error) {
	q := url.Values{}
	q.Set("uri", uri)
	if recursive {
		q.Set("recursive", "true")
	}
	q.Set("output", "original")
	raw, err := c.doRequest(ctx, "GET", "/api/v1/fs/ls", q, nil)
	if err != nil {
		return nil, err
	}
	var entries []VikingEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("viking: parse ls response: %w", err)
	}
	return entries, nil
}

func (c *vikingHTTPClient) Stat(ctx context.Context, uri string) (*VikingEntry, error) {
	q := url.Values{}
	q.Set("uri", uri)
	raw, err := c.doRequest(ctx, "GET", "/api/v1/fs/stat", q, nil)
	if err != nil {
		return nil, err
	}
	var entry VikingEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, fmt.Errorf("viking: parse stat response: %w", err)
	}
	return &entry, nil
}

func (c *vikingHTTPClient) Mkdir(ctx context.Context, uri string) error {
	_, err := c.doRequest(ctx, "POST", "/api/v1/fs/mkdir", nil, map[string]string{"uri": uri})
	return err
}

func (c *vikingHTTPClient) Remove(ctx context.Context, uri string, recursive bool) error {
	q := url.Values{}
	q.Set("uri", uri)
	if recursive {
		q.Set("recursive", "true")
	}
	_, err := c.doRequest(ctx, "DELETE", "/api/v1/fs", q, nil)
	return err
}

func (c *vikingHTTPClient) Move(ctx context.Context, fromURI, toURI string) error {
	_, err := c.doRequest(ctx, "POST", "/api/v1/fs/mv", nil, map[string]string{
		"from_uri": fromURI,
		"to_uri":   toURI,
	})
	return err
}

func (c *vikingHTTPClient) Read(ctx context.Context, uri string) (string, error) {
	q := url.Values{}
	q.Set("uri", uri)
	raw, err := c.doRequest(ctx, "GET", "/api/v1/content/read", q, nil)
	if err != nil {
		return "", err
	}
	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		return string(raw), nil
	}
	return content, nil
}

func (c *vikingHTTPClient) Abstract(ctx context.Context, uri string) (string, error) {
	q := url.Values{}
	q.Set("uri", uri)
	raw, err := c.doRequest(ctx, "GET", "/api/v1/content/abstract", q, nil)
	if err != nil {
		return "", err
	}
	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		return string(raw), nil
	}
	return content, nil
}

func (c *vikingHTTPClient) Overview(ctx context.Context, uri string) (string, error) {
	q := url.Values{}
	q.Set("uri", uri)
	raw, err := c.doRequest(ctx, "GET", "/api/v1/content/overview", q, nil)
	if err != nil {
		return "", err
	}
	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		return string(raw), nil
	}
	return content, nil
}

func (c *vikingHTTPClient) AddResource(ctx context.Context, path string, target string) (map[string]any, error) {
	body := map[string]any{
		"path": path,
		"wait": true,
	}
	if target != "" {
		body["target"] = target
	}
	raw, err := c.doRequest(ctx, "POST", "/api/v1/resources", nil, body)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("viking: parse add_resource response: %w", err)
	}
	return result, nil
}

func (c *vikingHTTPClient) Find(ctx context.Context, query string, targetURI string, limit int) ([]VikingSearchHit, error) {
	if limit <= 0 {
		limit = 10
	}
	body := map[string]any{
		"query": query,
		"limit": limit,
	}
	if targetURI != "" {
		body["target_uri"] = targetURI
	}
	raw, err := c.doRequest(ctx, "POST", "/api/v1/search/find", nil, body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Resources []VikingSearchHit `json:"resources"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		var hits []VikingSearchHit
		if err2 := json.Unmarshal(raw, &hits); err2 == nil {
			return hits, nil
		}
		return nil, fmt.Errorf("viking: parse find response: %w", err)
	}
	return result.Resources, nil
}

// ---------------------------------------------------------------------------
// VikingProvider — grasp Provider backed by an OpenViking server
// ---------------------------------------------------------------------------

var (
	_ types.Provider   = (*VikingProvider)(nil)
	_ types.Readable   = (*VikingProvider)(nil)
	_ types.Writable   = (*VikingProvider)(nil)
	_ types.Searchable = (*VikingProvider)(nil)
	_ types.Mutable    = (*VikingProvider)(nil)
)

// VikingProvider exposes an OpenViking context database as a grasp
// filesystem. Paths inside grasp map to viking:// URIs:
//
//	/ctx/resources/my_project  →  viking://resources/my_project
//
// Virtual files .abstract and .overview surface L0/L1 content tiers.
type VikingProvider struct {
	client  VikingClient
	baseURI string // e.g. "viking://" — prefix prepended to all paths
}

// NewVikingProvider creates a provider that connects to an OpenViking server.
// baseURI defaults to "viking://" if empty.
func NewVikingProvider(client VikingClient, baseURI string) *VikingProvider {
	if baseURI == "" {
		baseURI = "viking://"
	}
	if !strings.HasSuffix(baseURI, "/") {
		baseURI += "/"
	}
	return &VikingProvider{client: client, baseURI: baseURI}
}

func (p *VikingProvider) toURI(path string) string {
	path = normPath(path)
	if path == "" {
		return p.baseURI
	}
	return p.baseURI + path
}

func (p *VikingProvider) Stat(ctx context.Context, path string) (*types.Entry, error) {
	path = normPath(path)

	if path == "" {
		return &types.Entry{Name: "/", Path: "", IsDir: true, Perm: types.PermRWX}, nil
	}

	base := baseName(path)
	if base == ".abstract" || base == ".overview" {
		return &types.Entry{
			Name:     base,
			Path:     path,
			IsDir:    false,
			Perm:     types.PermRO,
			MimeType: "text/plain",
			Meta:     map[string]string{"kind": "viking-tier", "tier": strings.TrimPrefix(base, ".")},
		}, nil
	}

	ve, err := p.client.Stat(ctx, p.toURI(path))
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
		}
		return nil, err
	}
	return vikingToEntry(path, ve), nil
}

func (p *VikingProvider) List(ctx context.Context, path string, opts types.ListOpts) ([]types.Entry, error) {
	path = normPath(path)
	entries, err := p.client.Ls(ctx, p.toURI(path), opts.Recursive)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
		}
		return nil, err
	}

	result := make([]types.Entry, 0, len(entries)+2)
	hasDirEntries := false
	for _, ve := range entries {
		childPath := uriToRelPath(p.baseURI, ve.URI)
		e := vikingToEntry(childPath, &ve)
		result = append(result, *e)
		if ve.IsDir {
			hasDirEntries = true
		}
	}

	if hasDirEntries || path == "" {
		result = append(result,
			types.Entry{Name: ".abstract", Path: joinPath(path, ".abstract"), Perm: types.PermRO, MimeType: "text/plain", Meta: map[string]string{"kind": "viking-tier", "tier": "abstract"}},
			types.Entry{Name: ".overview", Path: joinPath(path, ".overview"), Perm: types.PermRO, MimeType: "text/plain", Meta: map[string]string{"kind": "viking-tier", "tier": "overview"}},
		)
	}

	return result, nil
}

func (p *VikingProvider) Open(ctx context.Context, path string) (types.File, error) {
	path = normPath(path)
	base := baseName(path)

	parentPath := dirPath(path)
	uri := p.toURI(parentPath)

	var content string
	var err error

	switch base {
	case ".abstract":
		content, err = p.client.Abstract(ctx, uri)
	case ".overview":
		content, err = p.client.Overview(ctx, uri)
	default:
		content, err = p.client.Read(ctx, p.toURI(path))
	}

	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
		}
		return nil, err
	}

	entry := &types.Entry{
		Name:     baseName(path),
		Path:     path,
		Perm:     types.PermRO,
		Size:     int64(len(content)),
		MimeType: "text/plain",
	}
	return types.NewFile(path, entry, io.NopCloser(strings.NewReader(content))), nil
}

// Write adds a resource to OpenViking. The content written is interpreted as a
// resource path or URL to ingest. For example:
//
//	echo "https://example.com/doc.md" > /ctx/resources/my_doc
func (p *VikingProvider) Write(ctx context.Context, path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	resourcePath := strings.TrimSpace(string(data))
	if resourcePath == "" {
		return fmt.Errorf("viking: empty resource path")
	}

	path = normPath(path)
	target := ""
	if path != "" {
		target = p.toURI(dirPath(path))
	}

	_, err = p.client.AddResource(ctx, resourcePath, target)
	return err
}

func (p *VikingProvider) Search(ctx context.Context, query string, opts types.SearchOpts) ([]types.SearchResult, error) {
	targetURI := ""
	if opts.Scope != "" {
		targetURI = p.toURI(opts.Scope)
	}
	limit := opts.MaxResults
	if limit <= 0 {
		limit = 10
	}

	hits, err := p.client.Find(ctx, query, targetURI, limit)
	if err != nil {
		return nil, err
	}

	results := make([]types.SearchResult, 0, len(hits))
	for _, h := range hits {
		relPath := uriToRelPath(p.baseURI, h.URI)
		snippet := h.Abstract
		if snippet == "" {
			snippet = h.Content
		}
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		results = append(results, types.SearchResult{
			Entry: types.Entry{
				Name: baseName(relPath),
				Path: relPath,
				Perm: types.PermRO,
				Meta: map[string]string{"kind": "viking-resource", "uri": h.URI},
			},
			Snippet: snippet,
			Score:   h.Score,
		})
	}
	return results, nil
}

func (p *VikingProvider) Mkdir(ctx context.Context, path string, _ types.Perm) error {
	path = normPath(path)
	return p.client.Mkdir(ctx, p.toURI(path))
}

func (p *VikingProvider) Remove(ctx context.Context, path string) error {
	path = normPath(path)
	return p.client.Remove(ctx, p.toURI(path), true)
}

func (p *VikingProvider) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPath = normPath(oldPath)
	newPath = normPath(newPath)
	return p.client.Move(ctx, p.toURI(oldPath), p.toURI(newPath))
}

func (p *VikingProvider) MountInfo() (string, string) {
	return "viking", p.baseURI
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func vikingToEntry(path string, ve *VikingEntry) *types.Entry {
	perm := types.PermRO
	if ve.IsDir {
		perm = types.PermRWX
	}

	meta := map[string]string{"kind": "viking-resource", "uri": ve.URI}
	if ve.ContextType != "" {
		meta["context_type"] = ve.ContextType
	}
	if ve.Abstract != "" {
		meta["abstract"] = ve.Abstract
	}

	name := ve.Name
	if name == "" {
		name = baseName(path)
	}

	var modified time.Time
	if ve.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, ve.UpdatedAt); err == nil {
			modified = t
		}
	}

	return &types.Entry{
		Name:     name,
		Path:     path,
		IsDir:    ve.IsDir,
		Perm:     perm,
		Size:     ve.Size,
		Modified: modified,
		Meta:     meta,
	}
}

func uriToRelPath(baseURI, uri string) string {
	rel := strings.TrimPrefix(uri, strings.TrimSuffix(baseURI, "/"))
	rel = strings.TrimPrefix(rel, "/")
	return rel
}

func dirPath(path string) string {
	idx := strings.LastIndexByte(path, '/')
	if idx < 0 {
		return ""
	}
	return path[:idx]
}

func joinPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not_found") || strings.Contains(msg, "not found")
}
