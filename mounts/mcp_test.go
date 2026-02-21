package mounts

import (
	"context"
	"io"
	"testing"

	"github.com/jackfish212/grasp/types"
)

// mockMCPClient implements MCPClient for testing
type mockMCPClient struct {
	tools     []MCPTool
	resources []MCPResource
	prompts   []MCPPrompt
}

func (m *mockMCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	return m.tools, nil
}

func (m *mockMCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error) {
	return &MCPToolResult{
		Content: []MCPContent{{Type: "text", Text: "tool result for " + name}},
	}, nil
}

func (m *mockMCPClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	return m.resources, nil
}

func (m *mockMCPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	return "resource content for " + uri, nil
}

func (m *mockMCPClient) ListPrompts(ctx context.Context) ([]MCPPrompt, error) {
	return m.prompts, nil
}

func (m *mockMCPClient) GetPrompt(ctx context.Context, name string, args map[string]any) (string, error) {
	return "prompt result for " + name, nil
}

func TestMCPToolProviderStat(t *testing.T) {
	client := &mockMCPClient{
		tools: []MCPTool{
			{Name: "test_tool", Description: "A test tool"},
		},
		prompts: []MCPPrompt{
			{Name: "test_prompt", Description: "A test prompt"},
		},
	}
	provider := NewMCPToolProvider(client)
	ctx := context.Background()

	// Test root
	entry, err := provider.Stat(ctx, "/")
	if err != nil {
		t.Fatalf("Stat(/) error: %v", err)
	}
	if !entry.IsDir {
		t.Error("root should be a directory")
	}

	// Test tool
	entry, err = provider.Stat(ctx, "test-tool")
	if err != nil {
		t.Fatalf("Stat(test-tool) error: %v", err)
	}
	if entry.Name != "test-tool" {
		t.Errorf("tool name = %q, want test-tool", entry.Name)
	}
	if entry.Meta["kind"] != "tool" {
		t.Errorf("tool kind = %q, want tool", entry.Meta["kind"])
	}

	// Test prompt
	entry, err = provider.Stat(ctx, "test-prompt")
	if err != nil {
		t.Fatalf("Stat(test-prompt) error: %v", err)
	}
	if entry.Meta["kind"] != "prompt" {
		t.Errorf("prompt kind = %q, want prompt", entry.Meta["kind"])
	}

	// Test not found
	_, err = provider.Stat(ctx, "nonexistent")
	if err == nil {
		t.Error("Stat(nonexistent) should fail")
	}
}

func TestMCPToolProviderList(t *testing.T) {
	client := &mockMCPClient{
		tools: []MCPTool{
			{Name: "tool_one", Description: "First tool"},
			{Name: "tool_two", Description: "Second tool"},
		},
		prompts: []MCPPrompt{
			{Name: "my_prompt", Description: "A prompt"},
		},
	}
	provider := NewMCPToolProvider(client)
	ctx := context.Background()

	entries, err := provider.List(ctx, "/", types.ListOpts{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("List returned %d entries, want 3", len(entries))
	}

	// Verify underscore to dash conversion
	found := make(map[string]bool)
	for _, e := range entries {
		found[e.Name] = true
	}
	if !found["tool-one"] {
		t.Error("missing tool-one")
	}
	if !found["tool-two"] {
		t.Error("missing tool-two")
	}
	if !found["my-prompt"] {
		t.Error("missing my-prompt")
	}

	// Test non-root should fail
	_, err = provider.List(ctx, "subdir", types.ListOpts{})
	if err == nil {
		t.Error("List on non-root should fail")
	}
}

func TestMCPToolProviderOpen(t *testing.T) {
	client := &mockMCPClient{
		tools: []MCPTool{
			{Name: "my_tool", Description: "A test tool", InputSchema: map[string]any{
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query"},
				},
			}},
		},
	}
	provider := NewMCPToolProvider(client)
	ctx := context.Background()

	f, err := provider.Open(ctx, "my-tool")
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer f.Close()

	data, _ := io.ReadAll(f)
	if len(data) == 0 {
		t.Error("Open returned empty content")
	}
}

func TestMCPToolProviderExec(t *testing.T) {
	client := &mockMCPClient{
		tools: []MCPTool{
			{Name: "echo_tool", InputSchema: map[string]any{
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
			}},
		},
	}
	provider := NewMCPToolProvider(client)
	ctx := context.Background()

	rc, err := provider.Exec(ctx, "echo-tool", []string{"--message", "hello"}, nil)
	if err != nil {
		t.Fatalf("Exec error: %v", err)
	}
	defer rc.Close()

	data, _ := io.ReadAll(rc)
	if len(data) == 0 {
		t.Error("Exec returned empty content")
	}
}

func TestMCPToolProviderSearch(t *testing.T) {
	client := &mockMCPClient{
		tools: []MCPTool{
			{Name: "search_tool", Description: "Search for items"},
			{Name: "other_tool", Description: "Different functionality"},
		},
		prompts: []MCPPrompt{
			{Name: "search_prompt", Description: "A search prompt"},
		},
	}
	provider := NewMCPToolProvider(client)
	ctx := context.Background()

	results, err := provider.Search(ctx, "search", types.SearchOpts{})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Search returned %d results, want 2", len(results))
	}
}

func TestMCPResourceProviderStat(t *testing.T) {
	client := &mockMCPClient{
		resources: []MCPResource{
			{URI: "file:///test.txt", Name: "test.txt", MimeType: "text/plain"},
		},
	}
	provider := NewMCPResourceProvider(client)
	ctx := context.Background()

	// Test root
	entry, err := provider.Stat(ctx, "/")
	if err != nil {
		t.Fatalf("Stat(/) error: %v", err)
	}
	if !entry.IsDir {
		t.Error("root should be a directory")
	}

	// Test resource
	entry, err = provider.Stat(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Stat(test.txt) error: %v", err)
	}
	if entry.Name != "test.txt" {
		t.Errorf("resource name = %q, want test.txt", entry.Name)
	}
}

func TestMCPResourceProviderList(t *testing.T) {
	client := &mockMCPClient{
		resources: []MCPResource{
			{URI: "file:///a.txt", Name: "a.txt"},
			{URI: "file:///b.txt", Name: "b.txt"},
		},
	}
	provider := NewMCPResourceProvider(client)
	ctx := context.Background()

	entries, err := provider.List(ctx, "/", types.ListOpts{})
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("List returned %d entries, want 2", len(entries))
	}
}

func TestMCPResourceProviderOpen(t *testing.T) {
	client := &mockMCPClient{
		resources: []MCPResource{
			{URI: "file:///data.json", Name: "data.json"},
		},
	}
	provider := NewMCPResourceProvider(client)
	ctx := context.Background()

	f, err := provider.Open(ctx, "data.json")
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer f.Close()

	data, _ := io.ReadAll(f)
	if len(data) == 0 {
		t.Error("Open returned empty content")
	}
}

func TestMCPResourceProviderSearch(t *testing.T) {
	client := &mockMCPClient{
		resources: []MCPResource{
			{Name: "config.json", Description: "Configuration file"},
			{Name: "data.txt", Description: "Data file"},
		},
	}
	provider := NewMCPResourceProvider(client)
	ctx := context.Background()

	results, err := provider.Search(ctx, "config", types.SearchOpts{})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search returned %d results, want 1", len(results))
	}
}

func TestFormatToolHelp(t *testing.T) {
	tool := MCPTool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]any{
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			},
			"required": []any{"query"},
		},
	}

	help := FormatToolHelp(tool)
	if help == "" {
		t.Error("FormatToolHelp returned empty string")
	}
	// Check underscore to dash conversion
	if !contains(help, "test-tool") {
		t.Error("help should contain 'test-tool'")
	}
	if !contains(help, "query") {
		t.Error("help should contain parameter 'query'")
	}
	if !contains(help, "[required]") {
		t.Error("help should contain [required] tag")
	}
}

func TestFormatPromptHelp(t *testing.T) {
	prompt := MCPPrompt{
		Name:        "my_prompt",
		Description: "A test prompt",
		ArgSchema: map[string]any{
			"properties": map[string]any{
				"topic": map[string]any{"type": "string"},
			},
		},
	}

	help := FormatPromptHelp(prompt)
	if help == "" {
		t.Error("FormatPromptHelp returned empty string")
	}
	if !contains(help, "my-prompt") {
		t.Error("help should contain 'my-prompt'")
	}
}

func TestParseCLIArgs(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"name":    map[string]any{"type": "string"},
			"count":   map[string]any{"type": "integer"},
			"numbers": map[string]any{"type": "array"},
			"verbose": map[string]any{"type": "boolean"},
		},
		"required": []any{"name"},
	}

	tests := []struct {
		name    string
		args    []string
		wantErr bool
		check   func(result map[string]any) bool
	}{
		{
			name: "string argument",
			args: []string{"--name", "test"},
			check: func(r map[string]any) bool {
				return r["name"] == "test"
			},
		},
		{
			name: "boolean flag",
			args: []string{"--name", "test", "--verbose"},
			check: func(r map[string]any) bool {
				return r["verbose"] == true
			},
		},
		{
			name: "array argument",
			args: []string{"--name", "test", "--numbers", "a,b,c"},
			check: func(r map[string]any) bool {
				arr, ok := r["numbers"].([]string)
				return ok && len(arr) == 3
			},
		},
		{
			name:    "missing required",
			args:    []string{"--verbose"},
			wantErr: true,
		},
		{
			name:    "missing value",
			args:    []string{"--name"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCLIArgs(tt.args, schema)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil && !tt.check(result) {
				t.Errorf("check failed for result: %v", result)
			}
		})
	}
}

func TestParseCLIArgsWithDashes(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"my_option": map[string]any{"type": "string"},
		},
		"required": []any{"my_option"},
	}

	result, err := ParseCLIArgs([]string{"--my-option", "value"}, schema)
	if err != nil {
		t.Fatalf("ParseCLIArgs error: %v", err)
	}
	if result["my_option"] != "value" {
		t.Errorf("my_option = %v, want 'value'", result["my_option"])
	}
}

func TestCliName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"test_tool", "test-tool"},
		{"my_long_name", "my-long-name"},
		{"nounderscore", "nounderscore"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := cliName(tt.input)
			if result != tt.expected {
				t.Errorf("cliName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResourceFileName(t *testing.T) {
	tests := []struct {
		name     string
		resource MCPResource
		expected string
	}{
		{
			name:     "with name",
			resource: MCPResource{Name: "myfile.txt", URI: "file:///path/to/file"},
			expected: "myfile.txt",
		},
		{
			name:     "without name",
			resource: MCPResource{URI: "file:///path/to/data.json"},
			expected: "data.json",
		},
		{
			name:     "empty URI",
			resource: MCPResource{URI: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resourceFileName(tt.resource)
			if result != tt.expected {
				t.Errorf("resourceFileName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMCPMountInfo(t *testing.T) {
	toolProvider := NewMCPToolProvider(&mockMCPClient{})
	name, _ := toolProvider.MountInfo()
	if name != "mcp" {
		t.Errorf("MCPToolProvider MountInfo name = %q, want mcp", name)
	}

	resourceProvider := NewMCPResourceProvider(&mockMCPClient{})
	name, extra := resourceProvider.MountInfo()
	if name != "mcp" {
		t.Errorf("MCPResourceProvider MountInfo name = %q, want mcp", name)
	}
	if extra == "" {
		t.Error("MountInfo extra should not be empty")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
