package mounts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/agentfs/afs/types"
)

// MCPClient abstracts the Model Context Protocol client.
type MCPClient interface {
	ListTools(ctx context.Context) ([]MCPTool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error)
	ListResources(ctx context.Context) ([]MCPResource, error)
	ReadResource(ctx context.Context, uri string) (string, error)
	ListPrompts(ctx context.Context) ([]MCPPrompt, error)
	GetPrompt(ctx context.Context, name string, args map[string]any) (string, error)
}

type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type MCPToolResult struct {
	Content []MCPContent
	IsError bool
}

type MCPContent struct {
	Type string
	Text string
}

type MCPResource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

type MCPPrompt struct {
	Name        string
	Description string
	ArgSchema   map[string]any
}

var (
	_ types.Provider   = (*MCPToolProvider)(nil)
	_ types.Readable   = (*MCPToolProvider)(nil)
	_ types.Executable = (*MCPToolProvider)(nil)
	_ types.Searchable = (*MCPToolProvider)(nil)
)

// MCPToolProvider exposes MCP tools and prompts as executable entries.
type MCPToolProvider struct {
	client  MCPClient
	tools   []MCPTool
	prompts []MCPPrompt
}

func NewMCPToolProvider(client MCPClient) *MCPToolProvider {
	return &MCPToolProvider{client: client}
}

func (p *MCPToolProvider) refresh(ctx context.Context) error {
	tools, err := p.client.ListTools(ctx)
	if err != nil {
		return err
	}
	p.tools = tools
	prompts, err := p.client.ListPrompts(ctx)
	if err != nil {
		p.prompts = nil
	} else {
		p.prompts = prompts
	}
	return nil
}

func (p *MCPToolProvider) ensureLoaded(ctx context.Context) error {
	if p.tools == nil && p.prompts == nil {
		return p.refresh(ctx)
	}
	return nil
}

func (p *MCPToolProvider) Stat(ctx context.Context, path string) (*types.Entry, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	path = normPath(path)
	if path == "" {
		return &types.Entry{Name: "/", Path: "", IsDir: true, Perm: types.PermRX}, nil
	}
	for _, t := range p.tools {
		if cliName(t.Name) == path {
			return &types.Entry{Name: cliName(t.Name), Path: path, Perm: types.PermRX, Meta: map[string]string{"kind": "tool", "description": t.Description}}, nil
		}
	}
	for _, pr := range p.prompts {
		if cliName(pr.Name) == path {
			return &types.Entry{Name: cliName(pr.Name), Path: path, Perm: types.PermRX, Meta: map[string]string{"kind": "prompt", "description": pr.Description}}, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
}

func (p *MCPToolProvider) List(ctx context.Context, path string, _ types.ListOpts) ([]types.Entry, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	if normPath(path) != "" {
		return nil, fmt.Errorf("%w: %s", types.ErrNotDir, path)
	}
	var entries []types.Entry
	for _, t := range p.tools {
		entries = append(entries, types.Entry{Name: cliName(t.Name), Path: cliName(t.Name), Perm: types.PermRX, Meta: map[string]string{"kind": "tool", "description": t.Description}})
	}
	for _, pr := range p.prompts {
		entries = append(entries, types.Entry{Name: cliName(pr.Name), Path: cliName(pr.Name), Perm: types.PermRX, Meta: map[string]string{"kind": "prompt", "description": pr.Description}})
	}
	return entries, nil
}

func (p *MCPToolProvider) Open(ctx context.Context, path string) (types.File, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	path = normPath(path)
	for _, t := range p.tools {
		if cliName(t.Name) == path {
			help := FormatToolHelp(t)
			entry := &types.Entry{Name: cliName(t.Name), Path: path, Perm: types.PermRX, Meta: map[string]string{"kind": "tool", "description": t.Description}}
			return types.NewFile(path, entry, io.NopCloser(strings.NewReader(help))), nil
		}
	}
	for _, pr := range p.prompts {
		if cliName(pr.Name) == path {
			help := FormatPromptHelp(pr)
			entry := &types.Entry{Name: cliName(pr.Name), Path: path, Perm: types.PermRX, Meta: map[string]string{"kind": "prompt", "description": pr.Description}}
			return types.NewFile(path, entry, io.NopCloser(strings.NewReader(help))), nil
		}
	}
	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
}

func (p *MCPToolProvider) Exec(ctx context.Context, path string, args []string, stdin io.Reader) (io.ReadCloser, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	path = normPath(path)

	for _, t := range p.tools {
		if cliName(t.Name) != path {
			continue
		}
		jsonArgs, err := ParseCLIArgs(args, t.InputSchema)
		if err != nil {
			help := FormatToolHelp(t)
			return io.NopCloser(strings.NewReader(fmt.Sprintf("error: %v\n\n%s", err, help))), nil
		}
		if stdin != nil {
			data, readErr := io.ReadAll(stdin)
			if readErr == nil && len(data) > 0 {
				jsonArgs["_stdin"] = string(data)
			}
		}
		result, err := p.client.CallTool(ctx, t.Name, jsonArgs)
		if err != nil {
			return nil, err
		}
		var buf strings.Builder
		for _, c := range result.Content {
			buf.WriteString(c.Text)
			buf.WriteByte('\n')
		}
		return io.NopCloser(strings.NewReader(buf.String())), nil
	}

	for _, pr := range p.prompts {
		if cliName(pr.Name) != path {
			continue
		}
		jsonArgs, err := ParseCLIArgs(args, pr.ArgSchema)
		if err != nil {
			help := FormatPromptHelp(pr)
			return io.NopCloser(strings.NewReader(fmt.Sprintf("error: %v\n\n%s", err, help))), nil
		}
		output, err := p.client.GetPrompt(ctx, pr.Name, jsonArgs)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(strings.NewReader(output + "\n")), nil
	}

	return nil, fmt.Errorf("%w: %s", types.ErrNotExecutable, path)
}

func (p *MCPToolProvider) Search(ctx context.Context, query string, _ types.SearchOpts) ([]types.SearchResult, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	lowerQuery := strings.ToLower(query)
	var results []types.SearchResult
	for _, t := range p.tools {
		if strings.Contains(strings.ToLower(t.Name), lowerQuery) || strings.Contains(strings.ToLower(t.Description), lowerQuery) {
			results = append(results, types.SearchResult{Entry: types.Entry{Name: cliName(t.Name), Path: cliName(t.Name), Perm: types.PermRX, Meta: map[string]string{"kind": "tool"}}, Snippet: t.Description, Score: 1.0})
		}
	}
	for _, pr := range p.prompts {
		if strings.Contains(strings.ToLower(pr.Name), lowerQuery) || strings.Contains(strings.ToLower(pr.Description), lowerQuery) {
			results = append(results, types.SearchResult{Entry: types.Entry{Name: cliName(pr.Name), Path: cliName(pr.Name), Perm: types.PermRX, Meta: map[string]string{"kind": "prompt"}}, Snippet: pr.Description, Score: 0.9})
		}
	}
	return results, nil
}

var (
	_ types.Provider   = (*MCPResourceProvider)(nil)
	_ types.Readable   = (*MCPResourceProvider)(nil)
	_ types.Searchable = (*MCPResourceProvider)(nil)
)

// MCPResourceProvider exposes MCP resources as readable file entries.
type MCPResourceProvider struct {
	client    MCPClient
	resources []MCPResource
}

func NewMCPResourceProvider(client MCPClient) *MCPResourceProvider {
	return &MCPResourceProvider{client: client}
}

func (p *MCPResourceProvider) refresh(ctx context.Context) error {
	resources, err := p.client.ListResources(ctx)
	if err != nil {
		return err
	}
	p.resources = resources
	return nil
}

func (p *MCPResourceProvider) ensureLoaded(ctx context.Context) error {
	if p.resources == nil {
		return p.refresh(ctx)
	}
	return nil
}

func (p *MCPResourceProvider) Stat(ctx context.Context, path string) (*types.Entry, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	path = normPath(path)
	if path == "" {
		return &types.Entry{Name: "/", Path: "", IsDir: true, Perm: types.PermRX}, nil
	}
	for _, r := range p.resources {
		if resourceFileName(r) == path {
			return &types.Entry{Name: resourceFileName(r), Path: path, Perm: types.PermRO, MimeType: r.MimeType, Meta: map[string]string{"kind": "resource", "uri": r.URI, "description": r.Description}}, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
}

func (p *MCPResourceProvider) List(ctx context.Context, path string, _ types.ListOpts) ([]types.Entry, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	if normPath(path) != "" {
		return nil, fmt.Errorf("%w: %s", types.ErrNotDir, path)
	}
	var entries []types.Entry
	for _, r := range p.resources {
		entries = append(entries, types.Entry{Name: resourceFileName(r), Path: resourceFileName(r), Perm: types.PermRO, MimeType: r.MimeType, Meta: map[string]string{"kind": "resource", "uri": r.URI}})
	}
	return entries, nil
}

func (p *MCPResourceProvider) Open(ctx context.Context, path string) (types.File, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	path = normPath(path)
	for _, r := range p.resources {
		if resourceFileName(r) == path {
			content, err := p.client.ReadResource(ctx, r.URI)
			if err != nil {
				return nil, err
			}
			entry := &types.Entry{Name: resourceFileName(r), Path: path, Perm: types.PermRO, MimeType: r.MimeType, Meta: map[string]string{"kind": "resource", "uri": r.URI}}
			return types.NewFile(path, entry, io.NopCloser(strings.NewReader(content))), nil
		}
	}
	return nil, fmt.Errorf("%w: %s", types.ErrNotFound, path)
}

func (p *MCPResourceProvider) Search(ctx context.Context, query string, _ types.SearchOpts) ([]types.SearchResult, error) {
	if err := p.ensureLoaded(ctx); err != nil {
		return nil, err
	}
	lowerQuery := strings.ToLower(query)
	var results []types.SearchResult
	for _, r := range p.resources {
		if strings.Contains(strings.ToLower(r.Name), lowerQuery) || strings.Contains(strings.ToLower(r.Description), lowerQuery) {
			results = append(results, types.SearchResult{Entry: types.Entry{Name: resourceFileName(r), Path: resourceFileName(r), Perm: types.PermRO, Meta: map[string]string{"kind": "resource"}}, Snippet: r.Description, Score: 0.8})
		}
	}
	return results, nil
}

// MountMCP registers an MCP server's tools+prompts and resources as separate providers.
func MountMCP(v interface{ Mount(string, types.Provider) error }, basePath string, client MCPClient) error {
	if err := v.Mount(basePath+"/tools", NewMCPToolProvider(client)); err != nil {
		return err
	}
	return v.Mount(basePath+"/data", NewMCPResourceProvider(client))
}

func cliName(name string) string        { return strings.ReplaceAll(name, "_", "-") }
func resourceFileName(r MCPResource) string {
	if r.Name != "" {
		return r.Name
	}
	parts := strings.Split(r.URI, "/")
	return parts[len(parts)-1]
}

func FormatToolHelp(t MCPTool) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "%s — %s\n", cliName(t.Name), t.Description)
	formatSchemaHelp(&buf, t.InputSchema)
	return buf.String()
}

func FormatPromptHelp(p MCPPrompt) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "%s — %s\n", cliName(p.Name), p.Description)
	formatSchemaHelp(&buf, p.ArgSchema)
	return buf.String()
}

func formatSchemaHelp(buf *strings.Builder, schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		buf.WriteString("\n(no parameters)\n")
		return
	}
	requiredSet := make(map[string]bool)
	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}
	buf.WriteString("\nParameters:\n")
	for name, prop := range props {
		p, ok := prop.(map[string]any)
		if !ok {
			continue
		}
		typStr, _ := p["type"].(string)
		desc, _ := p["description"].(string)
		flagName := strings.ReplaceAll(name, "_", "-")
		reqTag := ""
		if requiredSet[name] {
			reqTag = " [required]"
		}
		fmt.Fprintf(buf, "  --%s <%s>%s\n", flagName, typStr, reqTag)
		if desc != "" {
			fmt.Fprintf(buf, "      %s\n", desc)
		}
	}
}

func ParseCLIArgs(args []string, schema map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	props, _ := schema["properties"].(map[string]any)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		key := strings.TrimPrefix(arg, "--")
		key = strings.ReplaceAll(key, "-", "_")

		propType := "string"
		if props != nil {
			if p, ok := props[key].(map[string]any); ok {
				if t, ok := p["type"].(string); ok {
					propType = t
				}
			}
		}

		switch propType {
		case "boolean":
			result[key] = true
		default:
			if i+1 >= len(args) {
				return nil, fmt.Errorf("missing value for --%s", strings.ReplaceAll(key, "_", "-"))
			}
			i++
			val := args[i]
			switch propType {
			case "number", "integer":
				var n json.Number
				if err := json.Unmarshal([]byte(val), &n); err == nil {
					result[key] = n
				} else {
					result[key] = val
				}
			case "array":
				result[key] = strings.Split(val, ",")
			case "object":
				var obj map[string]any
				if err := json.Unmarshal([]byte(val), &obj); err == nil {
					result[key] = obj
				} else {
					result[key] = val
				}
			default:
				result[key] = val
			}
		}
	}

	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				if _, exists := result[s]; !exists {
					return nil, fmt.Errorf("missing required parameter: --%s", strings.ReplaceAll(s, "_", "-"))
				}
			}
		}
	}
	return result, nil
}

func (p *MCPToolProvider) MountInfo() (string, string)     { return "mcp", "MCP tools" }
func (p *MCPResourceProvider) MountInfo() (string, string) { return "mcp", "MCP resources" }
