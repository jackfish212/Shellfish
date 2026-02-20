package mcpserver

import "encoding/json"

const protocolVersion = "2024-11-05"

// ─── JSON-RPC 2.0 ───

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC error codes
const (
	errCodeParse          = -32700
	errCodeInvalidRequest = -32600
	errCodeMethodNotFound = -32601
	errCodeInvalidParams  = -32602
	errCodeInternal       = -32603
)

// ─── MCP Initialize ───

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
	Capabilities    any    `json:"capabilities"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version,omitempty"`
	} `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      serverInfo         `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type serverCapabilities struct {
	Tools *toolsCapability `json:"tools,omitempty"`
}

type toolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ─── MCP Tools ───

type toolsListResult struct {
	Tools []toolDef `json:"tools"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type toolsCallResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
