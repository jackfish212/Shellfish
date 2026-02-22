package httpfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ─── Declarative Schema ───

// HTTPFSSchema defines a declarative JSON configuration for bulk-adding
// HTTPFS sources. Useful for loading source definitions from config files.
//
// Example:
//
//	{
//	  "baseURL": "https://api.example.com",
//	  "defaults": { "headers": { "Authorization": "Bearer xxx" } },
//	  "sources": {
//	    "users":  { "path": "/users",  "parser": { "type": "json", "nameField": "name", "idField": "id" } },
//	    "feed":   { "url": "https://blog.example.com/rss", "parser": { "type": "rss" } },
//	    "status": { "path": "/health", "parser": { "type": "raw" } }
//	  }
//	}
type HTTPFSSchema struct {
	BaseURL  string                 `json:"baseURL,omitempty"`
	Defaults SchemaDefaults         `json:"defaults,omitempty"`
	Sources  map[string]SchemaSource `json:"sources"`
}

// SchemaDefaults are applied to all sources unless overridden.
type SchemaDefaults struct {
	Headers map[string]string `json:"headers,omitempty"`
}

// SchemaSource describes a single HTTP source.
type SchemaSource struct {
	// URL is an absolute URL. If empty, Path is resolved against BaseURL.
	URL string `json:"url,omitempty"`
	// Path is a relative path resolved against the schema's BaseURL.
	Path    string            `json:"path,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Parser  SchemaParser      `json:"parser,omitempty"`
}

// SchemaParser selects and configures a ResponseParser.
type SchemaParser struct {
	// Type selects the parser: "rss", "json", "raw", "auto" (default "auto").
	Type string `json:"type,omitempty"`
	// JSON parser options:
	ArrayField string `json:"arrayField,omitempty"`
	NameField  string `json:"nameField,omitempty"`
	IDField    string `json:"idField,omitempty"`
	// Raw parser options:
	Filename string `json:"filename,omitempty"`
}

// LoadSchema loads sources from a declarative JSON configuration.
// All configured sources are added to the HTTPFS; call Start to begin polling.
func (fs *HTTPFS) LoadSchema(data []byte) error {
	var schema HTTPFSSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}

	baseURL := strings.TrimRight(schema.BaseURL, "/")

	for name, src := range schema.Sources {
		url := src.URL
		if url == "" && src.Path != "" {
			url = baseURL + "/" + strings.TrimLeft(src.Path, "/")
		}
		if url == "" {
			return fmt.Errorf("source %q: missing url or path", name)
		}

		parser := buildParserFromSchema(src.Parser)

		var opts []SourceOption
		for k, v := range schema.Defaults.Headers {
			opts = append(opts, WithSourceHeader(k, v))
		}
		for k, v := range src.Headers {
			opts = append(opts, WithSourceHeader(k, v))
		}

		if err := fs.Add(name, url, parser, opts...); err != nil {
			return fmt.Errorf("source %q: %w", name, err)
		}
	}
	return nil
}

func buildParserFromSchema(sp SchemaParser) ResponseParser {
	switch sp.Type {
	case "rss":
		return &RSSParser{}
	case "json":
		return &JSONParser{
			ArrayField: sp.ArrayField,
			NameField:  sp.NameField,
			IDField:    sp.IDField,
		}
	case "raw":
		return &RawParser{Filename: sp.Filename}
	default:
		return &AutoParser{}
	}
}

// ─── OpenAPI 3.x ───

// LoadOpenAPI parses an OpenAPI 3.x specification (JSON) and creates sources
// for all GET endpoints that have no path parameters. The response schema is
// used to automatically configure an appropriate parser.
//
// Endpoints with path parameters (e.g., /users/{id}) are skipped because
// they require runtime values that cannot be polled generically.
//
// Schema $ref references are resolved within the spec's components/schemas.
func (fs *HTTPFS) LoadOpenAPI(spec []byte, opts ...SourceOption) error {
	var raw map[string]any
	if err := json.Unmarshal(spec, &raw); err != nil {
		return fmt.Errorf("invalid OpenAPI spec: %w", err)
	}

	var api openAPISpec
	if err := json.Unmarshal(spec, &api); err != nil {
		return fmt.Errorf("invalid OpenAPI spec: %w", err)
	}

	baseURL := ""
	if len(api.Servers) > 0 {
		baseURL = strings.TrimRight(api.Servers[0].URL, "/")
	}

	for path, item := range api.Paths {
		if item.Get == nil || strings.Contains(path, "{") {
			continue
		}

		name := openAPIPathToName(path)
		url := baseURL + path
		parser := inferParserFromOpenAPI(raw, item.Get)

		if err := fs.Add(name, url, parser, opts...); err != nil {
			return fmt.Errorf("endpoint %s: %w", path, err)
		}
	}
	return nil
}

// LoadOpenAPIFromURL fetches an OpenAPI spec from a URL and loads it.
func (fs *HTTPFS) LoadOpenAPIFromURL(ctx context.Context, specURL string, opts ...SourceOption) error {
	req, err := http.NewRequestWithContext(ctx, "GET", specURL, nil)
	if err != nil {
		return err
	}
	resp, err := fs.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch spec: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch spec: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return fs.LoadOpenAPI(data, opts...)
}

// ─── OpenAPI types (minimal subset) ───

type openAPISpec struct {
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Paths map[string]openAPIPathItem `json:"paths"`
}

type openAPIPathItem struct {
	Get *openAPIOperation `json:"get"`
}

type openAPIOperation struct {
	OperationID string                     `json:"operationId"`
	Summary     string                     `json:"summary"`
	Responses   map[string]openAPIResponse `json:"responses"`
}

type openAPIResponse struct {
	Content map[string]openAPIMediaType `json:"content"`
}

type openAPIMediaType struct {
	Schema openAPISchema `json:"schema"`
}

type openAPISchema struct {
	Type       string                   `json:"type"`
	Items      *openAPISchema           `json:"items"`
	Properties map[string]openAPISchema `json:"properties"`
	Ref        string                   `json:"$ref"`
}

// ─── OpenAPI helpers ───

// openAPIPathToName converts an API path to a filesystem-safe source name.
// /users → users, /api/v1/posts → api-v1-posts
func openAPIPathToName(path string) string {
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	var clean []string
	for _, p := range parts {
		if p != "" && !strings.HasPrefix(p, "{") {
			clean = append(clean, p)
		}
	}
	if len(clean) == 0 {
		return "root"
	}
	return strings.Join(clean, "-")
}

// inferParserFromOpenAPI examines the 200 response schema to pick a parser.
func inferParserFromOpenAPI(rawSpec map[string]any, op *openAPIOperation) ResponseParser {
	if op.Responses == nil {
		return &AutoParser{}
	}

	resp, ok := op.Responses["200"]
	if !ok {
		return &AutoParser{}
	}

	// Prefer application/json
	media, ok := resp.Content["application/json"]
	if !ok {
		for _, m := range resp.Content {
			media = m
			break
		}
	}

	schema := resolveOpenAPISchema(rawSpec, &media.Schema)
	if schema == nil {
		return &AutoParser{}
	}

	if schema.Type == "array" {
		jp := &JSONParser{}
		items := schema.Items
		if items != nil && items.Ref != "" {
			items = resolveOpenAPISchema(rawSpec, items)
		}
		if items != nil && items.Properties != nil {
			for _, f := range []string{"name", "title", "username", "label", "slug"} {
				if _, exists := items.Properties[f]; exists {
					jp.NameField = f
					break
				}
			}
			for _, f := range []string{"id", "_id", "uuid", "key"} {
				if _, exists := items.Properties[f]; exists {
					jp.IDField = f
					break
				}
			}
		}
		return jp
	}

	return &RawParser{}
}

// resolveOpenAPISchema follows a single level of $ref to resolve a schema.
// Supports the common pattern: $ref: "#/components/schemas/ModelName"
func resolveOpenAPISchema(rawSpec map[string]any, schema *openAPISchema) *openAPISchema {
	if schema == nil {
		return nil
	}
	if schema.Ref == "" {
		return schema
	}

	ref := schema.Ref
	if !strings.HasPrefix(ref, "#/") {
		return schema
	}

	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	var cur any = rawSpec
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return schema
		}
		cur = m[part]
	}

	resolved, err := json.Marshal(cur)
	if err != nil {
		return schema
	}
	var out openAPISchema
	if err := json.Unmarshal(resolved, &out); err != nil {
		return schema
	}
	return &out
}
