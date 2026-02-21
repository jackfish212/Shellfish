package mounts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// HttpMCPClient connects to an MCP server over HTTP (Streamable HTTP transport).
// It implements the MCPClient interface for use with MCPToolProvider and MCPResourceProvider.
//
// Supports the MCP Streamable HTTP transport (spec 2025-03-26+):
//   - JSON-RPC 2.0 over HTTP POST
//   - Session management via Mcp-Session-Id header
//   - Both application/json and text/event-stream responses
type HttpMCPClient struct {
	url        string
	httpClient *http.Client
	headers    map[string]string
	sessionID  string
	reqID      atomic.Int64
	mu         sync.Mutex
}

// HttpMCPOption configures an HttpMCPClient.
type HttpMCPOption func(*HttpMCPClient)

// WithHTTPClient sets a custom http.Client for the MCP connection.
func WithHTTPClient(client *http.Client) HttpMCPOption {
	return func(c *HttpMCPClient) { c.httpClient = client }
}

// WithHeader adds a custom header to all MCP requests.
func WithHeader(key, value string) HttpMCPOption {
	return func(c *HttpMCPClient) { c.headers[key] = value }
}

// WithBearerToken sets Bearer token authentication for all MCP requests.
func WithBearerToken(token string) HttpMCPOption {
	return WithHeader("Authorization", "Bearer "+token)
}

// NewHttpMCPClient creates a client that communicates with an MCP server
// via HTTP POST (Streamable HTTP transport).
func NewHttpMCPClient(url string, opts ...HttpMCPOption) *HttpMCPClient {
	c := &HttpMCPClient{
		url:        strings.TrimRight(url, "/"),
		httpClient: &http.Client{},
		headers:    make(map[string]string),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *HttpMCPClient) call(ctx context.Context, method string, params any) (*jsonRPCResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.reqID.Add(1)
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

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer httpResp.Body.Close()

	if sid := httpResp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.sessionID = sid
	}

	if httpResp.StatusCode == http.StatusAccepted {
		return &jsonRPCResponse{JSONRPC: "2.0"}, nil
	}

	if httpResp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("http %d: %s", httpResp.StatusCode, string(errBody))
	}

	contentType := httpResp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "text/event-stream") {
		return readSSEResponse(httpResp.Body)
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

func readSSEResponse(r io.Reader) (*jsonRPCResponse, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var lastData string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			lastData = strings.TrimPrefix(line, "data: ")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read SSE: %w", err)
	}
	if lastData == "" {
		return nil, fmt.Errorf("no data in SSE stream")
	}
	var resp jsonRPCResponse
	if err := json.Unmarshal([]byte(lastData), &resp); err != nil {
		return nil, fmt.Errorf("decode SSE data: %w", err)
	}
	return &resp, nil
}

func (c *HttpMCPClient) notify(ctx context.Context, method string) {
	notif := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
	}
	body, _ := json.Marshal(notif)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	if c.sessionID != "" {
		httpReq.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err == nil {
		resp.Body.Close()
	}
}

// Initialize performs the MCP handshake with the server.
func (c *HttpMCPClient) Initialize(ctx context.Context) (map[string]any, error) {
	params := map[string]any{
		"protocolVersion": "2025-03-26",
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
	c.notify(ctx, "notifications/initialized")
	result, _ := resp.Result.(map[string]any)
	return result, nil
}

// ListTools returns all available tools from the MCP server.
func (c *HttpMCPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}
	resultBytes, _ := json.Marshal(resp.Result)
	return parseToolsList(resultBytes)
}

// CallTool invokes a tool on the MCP server.
func (c *HttpMCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*MCPToolResult, error) {
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
	resultBytes, _ := json.Marshal(resp.Result)
	return parseToolCallResult(resultBytes)
}

// ListResources returns all available resources from the MCP server.
func (c *HttpMCPClient) ListResources(ctx context.Context) ([]MCPResource, error) {
	resp, err := c.call(ctx, "resources/list", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, nil
	}
	resultBytes, _ := json.Marshal(resp.Result)
	return parseResourcesList(resultBytes)
}

// ReadResource reads a resource from the MCP server.
func (c *HttpMCPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	params := map[string]any{"uri": uri}
	resp, err := c.call(ctx, "resources/read", params)
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("resources/read error: %s", resp.Error.Message)
	}
	resultBytes, _ := json.Marshal(resp.Result)
	return parseResourceRead(resultBytes)
}

// ListPrompts returns all available prompts from the MCP server.
func (c *HttpMCPClient) ListPrompts(ctx context.Context) ([]MCPPrompt, error) {
	resp, err := c.call(ctx, "prompts/list", nil)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, nil
	}
	resultBytes, _ := json.Marshal(resp.Result)
	return parsePromptsList(resultBytes)
}

// GetPrompt retrieves a prompt template from the MCP server.
func (c *HttpMCPClient) GetPrompt(ctx context.Context, name string, args map[string]any) (string, error) {
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
	resultBytes, _ := json.Marshal(resp.Result)
	return parsePromptGet(resultBytes)
}
