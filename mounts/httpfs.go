package mounts

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jackfish212/shellfish/types"
)

var (
	_ types.Provider          = (*HTTPFS)(nil)
	_ types.Readable          = (*HTTPFS)(nil)
	_ types.Writable          = (*HTTPFS)(nil)
	_ types.Mutable           = (*HTTPFS)(nil)
	_ types.MountInfoProvider = (*HTTPFS)(nil)
)

// ─── ResponseParser interface ───

// ResponseParser transforms an HTTP response body into virtual files.
// Implement this interface to support custom response formats.
type ResponseParser interface {
	Parse(body []byte) ([]ParsedFile, error)
}

// ParsedFile represents a single file produced by parsing an HTTP response.
type ParsedFile struct {
	Name    string    // display name (will be slugified for the filename)
	Content string    // file content
	ModTime time.Time // modification time (zero value defaults to fetch time)
	ID      string    // unique identifier for dedup; empty falls back to Name
}

// ─── HTTPFS ───

// HTTPFS maps HTTP endpoints to a virtual filesystem with automatic polling.
// Each source is an HTTP URL paired with a ResponseParser that transforms
// the response body into virtual files.
//
// Filesystem layout:
//
//	(root)               — directory listing all sources
//	<name>/              — source directory containing parsed files
//	<name>/<slug>.txt    — a single parsed file (read-only)
//
// Adding sources:
//
//	Go API:  fs.Add("name", "https://...", &RSSParser{})
//	Shell:   echo "https://..." > /mount/name   (uses AutoParser)
//
// Removing sources:
//
//	Go API:  fs.RemoveSource("name")
//	Shell:   rm /mount/name
type HTTPFS struct {
	mu       sync.RWMutex
	sources  map[string]*httpSource
	client   *http.Client
	interval time.Duration
	onEvent  func(types.EventType, string)
	cancel   context.CancelFunc
	runCtx   context.Context
	wg       sync.WaitGroup
}

type httpSource struct {
	name     string
	url      string
	parser   ResponseParser
	headers  map[string]string
	files    []*fileEntry
	fileIdx  map[string]*fileEntry // slug → entry
	idToSlug map[string]string     // parsed ID → slug
	etag     string
	lastMod  string
	updated  time.Time
}

type fileEntry struct {
	slug    string
	content string
	modTime time.Time
}

// HTTPFSOption configures an HTTPFS instance.
type HTTPFSOption func(*HTTPFS)

// WithHTTPFSClient sets a custom HTTP client for fetching sources.
func WithHTTPFSClient(c *http.Client) HTTPFSOption {
	return func(fs *HTTPFS) { fs.client = c }
}

// WithHTTPFSInterval sets the default polling interval (default 5 minutes).
func WithHTTPFSInterval(d time.Duration) HTTPFSOption {
	return func(fs *HTTPFS) { fs.interval = d }
}

// WithHTTPFSOnEvent sets a callback invoked on file changes.
// Path is relative to the provider root (e.g., "sourcename/file.txt").
// Wire this to VirtualOS.Notify to propagate events through the watch system.
func WithHTTPFSOnEvent(fn func(types.EventType, string)) HTTPFSOption {
	return func(fs *HTTPFS) { fs.onEvent = fn }
}

// SourceOption configures an individual source.
type SourceOption func(*httpSource)

// WithSourceHeader adds a custom HTTP header to requests for this source.
func WithSourceHeader(key, value string) SourceOption {
	return func(s *httpSource) {
		if s.headers == nil {
			s.headers = make(map[string]string)
		}
		s.headers[key] = value
	}
}

// NewHTTPFS creates a new HTTP filesystem provider.
func NewHTTPFS(opts ...HTTPFSOption) *HTTPFS {
	fs := &HTTPFS{
		sources:  make(map[string]*httpSource),
		client:   &http.Client{Timeout: 30 * time.Second},
		interval: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(fs)
	}
	return fs
}

// Add registers an HTTP source with a specific parser.
// If the provider is already started, polling begins immediately.
func (fs *HTTPFS) Add(name, url string, parser ResponseParser, opts ...SourceOption) error {
	fs.mu.Lock()
	if _, ok := fs.sources[name]; ok {
		fs.mu.Unlock()
		return fmt.Errorf("source %q already exists", name)
	}
	src := newHTTPSource(name, url, parser)
	for _, opt := range opts {
		opt(src)
	}
	fs.sources[name] = src
	ctx := fs.runCtx
	fs.mu.Unlock()

	if ctx != nil {
		go fs.fetchSource(ctx, name)
		fs.startSourcePoll(ctx, name)
	}
	return nil
}

// RemoveSource unsubscribes from a source by name.
func (fs *HTTPFS) RemoveSource(name string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.removeLocked(name)
}

func (fs *HTTPFS) removeLocked(name string) error {
	if _, ok := fs.sources[name]; !ok {
		return fmt.Errorf("source %q not found", name)
	}
	delete(fs.sources, name)
	return nil
}

// Sources returns a snapshot of all source names and their URLs.
func (fs *HTTPFS) Sources() map[string]string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	result := make(map[string]string, len(fs.sources))
	for name, src := range fs.sources {
		result[name] = src.url
	}
	return result
}

// Start begins background polling of all sources.
// The initial fetch is synchronous so data is available immediately.
func (fs *HTTPFS) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	fs.mu.Lock()
	fs.cancel = cancel
	fs.runCtx = ctx
	names := make([]string, 0, len(fs.sources))
	for n := range fs.sources {
		names = append(names, n)
	}
	fs.mu.Unlock()

	fs.fetchAll(ctx)

	for _, name := range names {
		fs.startSourcePoll(ctx, name)
	}
}

// Stop terminates all background polling and waits for goroutines to finish.
func (fs *HTTPFS) Stop() {
	fs.mu.Lock()
	if fs.cancel != nil {
		fs.cancel()
	}
	fs.runCtx = nil
	fs.mu.Unlock()
	fs.wg.Wait()
}

// startSourcePoll launches a per-source polling goroutine.
// The goroutine exits when the context is cancelled or the source is removed.
func (fs *HTTPFS) startSourcePoll(ctx context.Context, name string) {
	interval := fs.interval
	fs.wg.Add(1)
	go func() {
		defer fs.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fs.mu.RLock()
				_, exists := fs.sources[name]
				fs.mu.RUnlock()
				if !exists {
					return
				}
				fs.fetchSource(ctx, name)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// ─── Provider ───

func (fs *HTTPFS) Stat(_ context.Context, path string) (*types.Entry, error) {
	path = normPath(path)
	if path == "" {
		return &types.Entry{Name: "http", IsDir: true, Perm: types.PermRW}, nil
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	parts := strings.SplitN(path, "/", 2)
	src, ok := fs.sources[parts[0]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	if len(parts) == 1 {
		return src.toEntry(), nil
	}
	fe, ok := src.fileIdx[parts[1]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	return fe.toEntry(), nil
}

func (fs *HTTPFS) List(_ context.Context, path string, _ types.ListOpts) ([]types.Entry, error) {
	path = normPath(path)

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if path == "" {
		entries := make([]types.Entry, 0, len(fs.sources))
		for _, src := range fs.sources {
			entries = append(entries, *src.toEntry())
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		return entries, nil
	}

	parts := strings.SplitN(path, "/", 2)
	src, ok := fs.sources[parts[0]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	if len(parts) > 1 {
		return nil, fmt.Errorf("%w: %s", types.ErrNotDir, path)
	}
	entries := make([]types.Entry, len(src.files))
	for i, fe := range src.files {
		entries[i] = *fe.toEntry()
	}
	return entries, nil
}

// ─── Readable ───

func (fs *HTTPFS) Open(_ context.Context, path string) (types.File, error) {
	path = normPath(path)

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: %s", types.ErrIsDir, path)
	}
	src, ok := fs.sources[parts[0]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	fe, ok := src.fileIdx[parts[1]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
	}
	entry := fe.toEntry()
	return types.NewFile(path, entry, io.NopCloser(strings.NewReader(fe.content))), nil
}

// ─── Writable (subscribe via shell: echo URL > /mount/name) ───

func (fs *HTTPFS) Write(_ context.Context, path string, r io.Reader) error {
	path = normPath(path)
	if strings.Contains(path, "/") || path == "" {
		return fmt.Errorf("%w: %s (write a URL to a source name to subscribe)", types.ErrNotWritable, path)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	url := strings.TrimSpace(string(data))
	if url == "" {
		return fmt.Errorf("empty URL")
	}

	isNew := false
	fs.mu.Lock()
	if src, ok := fs.sources[path]; ok {
		src.url = url
		src.files = nil
		src.fileIdx = make(map[string]*fileEntry)
		src.idToSlug = make(map[string]string)
	} else {
		fs.sources[path] = newHTTPSource(path, url, &AutoParser{})
		isNew = true
	}
	ctx := fs.runCtx
	fs.mu.Unlock()

	if ctx != nil {
		if isNew {
			fs.startSourcePoll(ctx, path)
		}
		go fs.fetchSource(ctx, path)
	}
	return nil
}

// ─── Mutable ───

func (fs *HTTPFS) Mkdir(_ context.Context, _ string, _ types.Perm) error {
	return fmt.Errorf("%w: use write to add a source", types.ErrNotSupported)
}

func (fs *HTTPFS) Remove(_ context.Context, path string) error {
	path = normPath(path)
	if strings.Contains(path, "/") || path == "" {
		return fmt.Errorf("%w: can only remove sources, not individual files", types.ErrNotSupported)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.removeLocked(path)
}

func (fs *HTTPFS) Rename(_ context.Context, _, _ string) error {
	return fmt.Errorf("%w: rename not supported", types.ErrNotSupported)
}

func (fs *HTTPFS) MountInfo() (string, string) {
	fs.mu.RLock()
	n := len(fs.sources)
	fs.mu.RUnlock()
	return "httpfs", fmt.Sprintf("%d sources", n)
}

// ─── Polling ───

func (fs *HTTPFS) fetchAll(ctx context.Context) {
	fs.mu.RLock()
	names := make([]string, 0, len(fs.sources))
	for n := range fs.sources {
		names = append(names, n)
	}
	fs.mu.RUnlock()

	var wg sync.WaitGroup
	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			fs.fetchSource(ctx, n)
		}(name)
	}
	wg.Wait()
}

func (fs *HTTPFS) fetchSource(ctx context.Context, name string) {
	fs.mu.RLock()
	src, ok := fs.sources[name]
	if !ok {
		fs.mu.RUnlock()
		return
	}
	srcURL := src.url
	etag := src.etag
	lastModHdr := src.lastMod
	parser := src.parser
	var headers map[string]string
	if len(src.headers) > 0 {
		headers = make(map[string]string, len(src.headers))
		for k, v := range src.headers {
			headers[k] = v
		}
	}
	fs.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", srcURL, nil)
	if err != nil {
		return
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModHdr != "" {
		req.Header.Set("If-Modified-Since", lastModHdr)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := fs.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return
	}
	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	parsed, err := parser.Parse(body)
	if err != nil || len(parsed) == 0 {
		return
	}

	fs.mu.Lock()
	src, ok = fs.sources[name]
	if !ok {
		fs.mu.Unlock()
		return
	}
	src.etag = resp.Header.Get("ETag")
	src.lastMod = resp.Header.Get("Last-Modified")
	src.updated = time.Now()

	var newPaths, updatedPaths []string
	for _, pf := range parsed {
		id := pf.ID
		if id == "" {
			id = pf.Name
		}
		modTime := pf.ModTime
		if modTime.IsZero() {
			modTime = time.Now()
		}

		if existingSlug, known := src.idToSlug[id]; known {
			if fe := src.fileIdx[existingSlug]; fe != nil && fe.content != pf.Content {
				fe.content = pf.Content
				fe.modTime = modTime
				updatedPaths = append(updatedPaths, name+"/"+existingSlug)
			}
			continue
		}

		slug := makeSlug(pf.Name) + ".txt"
		base := slug[:len(slug)-4]
		for i := 2; src.fileIdx[slug] != nil; i++ {
			slug = fmt.Sprintf("%s-%d.txt", base, i)
		}

		fe := &fileEntry{slug: slug, content: pf.Content, modTime: modTime}
		src.fileIdx[slug] = fe
		src.idToSlug[id] = slug
		src.files = append(src.files, fe)
		newPaths = append(newPaths, name+"/"+slug)
	}
	fs.mu.Unlock()

	if fs.onEvent != nil {
		for _, p := range newPaths {
			fs.onEvent(types.EventCreate, p)
		}
		for _, p := range updatedPaths {
			fs.onEvent(types.EventWrite, p)
		}
	}
}

// ─── Built-in Parsers ───

// RSSParser parses RSS 2.0 and Atom feeds into individual item files.
// Each item becomes a ParsedFile with a formatted text content containing
// title, link, date, and description.
type RSSParser struct{}

func (RSSParser) Parse(body []byte) ([]ParsedFile, error) {
	clean := cleanXMLNamespaces(body)
	if items := tryParseRSS(clean); len(items) > 0 {
		return items, nil
	}
	if items := tryParseAtom(clean); len(items) > 0 {
		return items, nil
	}
	return nil, fmt.Errorf("not a valid RSS or Atom feed")
}

// JSONParser parses JSON responses into individual files.
// Supports both root-level arrays and nested arrays via ArrayField.
type JSONParser struct {
	// ArrayField is the dot-separated path to the JSON array.
	// Empty string means the root value is the array.
	ArrayField string

	// NameField is the object field used for file naming.
	// Falls back to "item-N" if not set or the field doesn't exist.
	NameField string

	// IDField is the object field used for dedup.
	// Falls back to NameField if not set.
	IDField string
}

func (p *JSONParser) Parse(body []byte) ([]ParsedFile, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var items []any
	if p.ArrayField == "" {
		arr, ok := raw.([]any)
		if !ok {
			items = []any{raw}
		} else {
			items = arr
		}
	} else {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected JSON object for ArrayField navigation")
		}
		val := jsonNavigate(obj, p.ArrayField)
		arr, ok := val.([]any)
		if !ok {
			return nil, fmt.Errorf("field %q is not a JSON array", p.ArrayField)
		}
		items = arr
	}

	files := make([]ParsedFile, 0, len(items))
	for i, item := range items {
		name := fmt.Sprintf("item-%d", i)
		id := ""

		if obj, ok := item.(map[string]any); ok {
			if p.NameField != "" {
				if v, exists := obj[p.NameField]; exists {
					name = fmt.Sprintf("%v", v)
				}
			}
			idField := p.IDField
			if idField == "" {
				idField = p.NameField
			}
			if idField != "" {
				if v, exists := obj[idField]; exists {
					id = fmt.Sprintf("%v", v)
				}
			}
		}

		if id == "" {
			id = name
		}

		content, _ := json.MarshalIndent(item, "", "  ")
		files = append(files, ParsedFile{
			Name:    name,
			Content: string(content),
			ID:      id,
		})
	}
	return files, nil
}

func jsonNavigate(obj map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var cur any = obj
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

// RawParser returns the entire response body as a single file.
type RawParser struct {
	Filename string // base name for the file (default "content")
}

func (p *RawParser) Parse(body []byte) ([]ParsedFile, error) {
	name := p.Filename
	if name == "" {
		name = "content"
	}
	return []ParsedFile{{
		Name:    name,
		Content: string(body),
		ID:      "_raw",
		ModTime: time.Now(),
	}}, nil
}

// AutoParser tries RSS/Atom first, then falls back to raw content.
// Used by default when sources are added via shell (echo URL > /mount/name).
type AutoParser struct{}

func (AutoParser) Parse(body []byte) ([]ParsedFile, error) {
	if files, err := (RSSParser{}).Parse(body); err == nil && len(files) > 0 {
		return files, nil
	}
	return (&RawParser{}).Parse(body)
}

// ─── RSS/Atom XML internals ───

var (
	reXMLNS    = regexp.MustCompile(`\sxmlns(?::\w+)?="[^"]*"`)
	reXMLPrefix = regexp.MustCompile(`<(/?)(\w+):(\w+)`)
)

// cleanXMLNamespaces strips namespace declarations and element prefixes
// so we can parse RSS/Atom with simple struct tags.
func cleanXMLNamespaces(data []byte) []byte {
	data = reXMLNS.ReplaceAll(data, nil)
	data = reXMLPrefix.ReplaceAll(data, []byte("<${1}${3}"))
	return data
}

type rssDoc struct {
	Channel struct {
		Items []rssItemXML `xml:"item"`
	} `xml:"channel"`
}

type rssItemXML struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Encoded     string `xml:"encoded"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

type atomDoc struct {
	Entries []atomEntryXML `xml:"entry"`
}

type atomEntryXML struct {
	Title     string        `xml:"title"`
	Links     []atomLinkXML `xml:"link"`
	Summary   string        `xml:"summary"`
	Content   string        `xml:"content"`
	Updated   string        `xml:"updated"`
	Published string        `xml:"published"`
	ID        string        `xml:"id"`
}

type atomLinkXML struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func tryParseRSS(data []byte) []ParsedFile {
	var doc rssDoc
	if err := xml.Unmarshal(data, &doc); err != nil || len(doc.Channel.Items) == 0 {
		return nil
	}
	files := make([]ParsedFile, len(doc.Channel.Items))
	for i, x := range doc.Channel.Items {
		desc := x.Description
		if desc == "" {
			desc = x.Encoded
		}
		pubDate := parseHTTPDate(x.PubDate)
		files[i] = ParsedFile{
			Name:    x.Title,
			Content: formatRSSEntry(x.Title, x.Link, pubDate, desc),
			ModTime: pubDate,
			ID:      firstNonEmpty(x.GUID, x.Link, x.Title),
		}
	}
	return files
}

func tryParseAtom(data []byte) []ParsedFile {
	var doc atomDoc
	if err := xml.Unmarshal(data, &doc); err != nil || len(doc.Entries) == 0 {
		return nil
	}
	files := make([]ParsedFile, len(doc.Entries))
	for i, x := range doc.Entries {
		link := ""
		for _, l := range x.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				link = l.Href
				break
			}
		}
		if link == "" && len(x.Links) > 0 {
			link = x.Links[0].Href
		}
		desc := x.Summary
		if desc == "" {
			desc = x.Content
		}
		dateStr := x.Published
		if dateStr == "" {
			dateStr = x.Updated
		}
		pubDate := parseHTTPDate(dateStr)
		files[i] = ParsedFile{
			Name:    x.Title,
			Content: formatRSSEntry(x.Title, link, pubDate, desc),
			ModTime: pubDate,
			ID:      firstNonEmpty(x.ID, link, x.Title),
		}
	}
	return files
}

func formatRSSEntry(title, link string, pubDate time.Time, desc string) string {
	var b strings.Builder
	if title != "" {
		fmt.Fprintf(&b, "Title: %s\n", title)
	}
	if link != "" {
		fmt.Fprintf(&b, "Link: %s\n", link)
	}
	if !pubDate.IsZero() {
		fmt.Fprintf(&b, "Date: %s\n", pubDate.Format(time.RFC3339))
	}
	b.WriteByte('\n')
	if desc != "" {
		b.WriteString(desc)
		if !strings.HasSuffix(desc, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

var httpDateFormats = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC3339,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05Z",
	"2006-01-02",
}

func parseHTTPDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, f := range httpDateFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// ─── Helpers ───

func newHTTPSource(name, url string, parser ResponseParser) *httpSource {
	return &httpSource{
		name:     name,
		url:      url,
		parser:   parser,
		fileIdx:  make(map[string]*fileEntry),
		idToSlug: make(map[string]string),
	}
}

func (src *httpSource) toEntry() *types.Entry {
	return &types.Entry{
		Name:     src.name,
		IsDir:    true,
		Perm:     types.PermRO,
		Modified: src.updated,
		Meta: map[string]string{
			"url":   src.url,
			"files": fmt.Sprintf("%d", len(src.files)),
		},
	}
}

func (f *fileEntry) toEntry() *types.Entry {
	return &types.Entry{
		Name:     f.slug,
		Perm:     types.PermRO,
		Size:     int64(len(f.content)),
		Modified: f.modTime,
	}
}

func makeSlug(title string) string {
	var buf strings.Builder
	lastSep := true
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(r)
			lastSep = false
		} else if !lastSep {
			buf.WriteByte('-')
			lastSep = true
		}
	}
	s := strings.Trim(buf.String(), "-")
	rs := []rune(s)
	if len(rs) > 60 {
		rs = rs[:60]
		s = string(rs)
		if i := strings.LastIndex(s, "-"); i > len(s)/2 {
			s = s[:i]
		}
	}
	if s == "" {
		s = "untitled"
	}
	return s
}
