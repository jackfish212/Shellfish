package mounts

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// StdioMCPClient connects to an MCP server over stdio (subprocess).
// It implements the MCPClient interface for use with MCPToolProvider.
type StdioMCPClient struct {
	cmdIn  io.Writer
	cmdOut io.Reader
	reqID  atomic.Int64
	mu     sync.Mutex
}

// NewStdioMCPClient creates a client that communicates with an MCP server
// via the provided stdin/stdout streams.
func NewStdioMCPClient(stdin io.Writer, stdout io.Reader) *StdioMCPClient {
	return &StdioMCPClient{
		cmdIn:  stdin,
		cmdOut: stdout,
	}
}

func (c *StdioMCPClient) nextID() int64 {
	return c.reqID.Add(1)
}

// jsonRPCRequest represents a JSON-RPC 2.0 request
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse represents a JSON-RPC 2.0 response
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError represents a JSON-RPC 2.0 error
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (c *StdioMCPClient) call(ctx context.Context, method string, params any) (*jsonRPCResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID()
	idBytes, _ := json.Marshal(id)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      idBytes,
		Method:  method,
	}
	if params != nil {
		p, _ := json.Marshal(params)
		req.Params = p
	}

	// Write request
	if err := json.NewEncoder(c.cmdIn).Encode(req); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	// Read response
	scanner := bufio.NewScanner(c.cmdOut)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, fmt.Errorf("no response received")
	}

	var resp jsonRPCResponse
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &resp, nil
}

// Initialize performs the MCP handshake with the server.
func (c *StdioMCPClient) Initialize(ctx context.Context) (map[string]any, error) {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "grasp-mcp-client",
			"version": "1.0.0",
		},
	}
	resp, err := c.call(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	// Send initialized notification
	c.mu.Lock()
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	_ = json.NewEncoder(c.cmdIn).Encode(notif)
	c.mu.Unlock()

	result, _ := resp.Result.(map[string]any)
	return result, nil
}

// ListTools returns all available tools from the MCP server.
func (c *StdioMCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}

	var result struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description,omitempty"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("parse tools: %w", err)
	}

	tools := make([]MCPTool, len(result.Tools))
	for i, t := range result.Tools {
		tools[i] = MCPTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *StdioMCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	resp, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/call error: %s", resp.Error.Message)
	}

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text,omitempty"`
		} `json:"content"`
		IsError bool `json:"isError,omitempty"`
	}
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("parse tool result: %w", err)
	}

	content := make([]MCPContent, len(result.Content))
	for i, c := range result.Content {
		content[i] = MCPContent{Type: c.Type, Text: c.Text}
	}
	return &MCPToolResult{Content: content, IsError: result.IsError}, nil
}

// ListResources returns all available resources from the MCP server.
func (c *StdioMCPClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	resp, err := c.call(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		// Resources may not be supported
		return nil, nil
	}

	var result struct {
		Resources []struct {
			URI         string `json:"uri"`
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			MimeType    string `json:"mimeType,omitempty"`
		} `json:"resources"`
	}
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, nil
	}

	resources := make([]MCPResource, len(result.Resources))
	for i, r := range result.Resources {
		resources[i] = MCPResource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
		}
	}
	return resources, nil
}

// ReadResource reads a resource from the MCP server.
func (c *StdioMCPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	params := map[string]any{"uri": uri}
	resp, err := c.call(ctx, "resources/read", params)
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("resources/read error: %s", resp.Error.Message)
	}

	var result struct {
		Contents []struct {
			URI      string `json:"uri"`
			Text     string `json:"text,omitempty"`
			MimeType string `json:"mimeType,omitempty"`
		} `json:"contents"`
	}
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return "", nil
	}
	if len(result.Contents) > 0 {
		return result.Contents[0].Text, nil
	}
	return "", nil
}

// ListPrompts returns all available prompts from the MCP server.
func (c *StdioMCPClient) ListPrompts(ctx context.Context) ([]MCPPrompt, error) {
	resp, err := c.call(ctx, "prompts/list", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		// Prompts may not be supported
		return nil, nil
	}

	var result struct {
		Prompts []struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
		} `json:"prompts"`
	}
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, nil
	}

	prompts := make([]MCPPrompt, len(result.Prompts))
	for i, p := range result.Prompts {
		prompts[i] = MCPPrompt{
			Name:        p.Name,
			Description: p.Description,
		}
	}
	return prompts, nil
}

// GetPrompt retrieves a prompt template from the MCP server.
func (c *StdioMCPClient) GetPrompt(ctx context.Context, name string, args map[string]any) (string, error) {
	params := map[string]any{
		"name":      name,
		"arguments": args,
	}
	resp, err := c.call(ctx, "prompts/get", params)
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("prompts/get error: %s", resp.Error.Message)
	}

	var result struct {
		Messages []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text,omitempty"`
			} `json:"content"`
		} `json:"messages"`
	}
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return "", nil
	}

	var text string
	for _, m := range result.Messages {
		if m.Content.Type == "text" {
			text += m.Content.Text + "\n"
		}
	}
	return text, nil
}
