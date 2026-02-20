package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/builtins"
	"github.com/jackfish212/shellfish/mounts"
)

func setupTestServer(t *testing.T) *Server {
	t.Helper()
	v := shellfish.New()
	rootFS, err := shellfish.Configure(v)
	if err != nil {
		t.Fatalf("Configure: %v", err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	mem := mounts.NewMemFS(shellfish.PermRW)
	mem.AddFile("hello.txt", []byte("Hello, Shellfish!\n"), shellfish.PermRW)
	mem.AddDir("subdir")
	mem.AddFile("subdir/nested.txt", []byte("nested content\n"), shellfish.PermRW)
	if err := v.Mount("/data", mem); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	return New(v, "test")
}

func roundTrip(t *testing.T, srv *Server, method string, id int, params any) jsonRPCResponse {
	t.Helper()

	var paramsJSON json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			t.Fatalf("marshal params: %v", err)
		}
		paramsJSON = b
	}
	idJSON, _ := json.Marshal(id)

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      idJSON,
		Method:  method,
		Params:  paramsJSON,
	}
	line, _ := json.Marshal(req)
	line = append(line, '\n')

	var out bytes.Buffer
	in := bytes.NewReader(line)

	ctx := context.Background()
	if err := srv.Run(ctx, in, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var resp jsonRPCResponse
	if err := json.NewDecoder(&out).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v (raw: %s)", err, out.String())
	}
	return resp
}

func TestInitialize(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "initialize", 1, map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "1.0"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	b, _ := json.Marshal(resp.Result)
	var result initializeResult
	json.Unmarshal(b, &result)

	if result.ProtocolVersion != protocolVersion {
		t.Errorf("protocolVersion = %q, want %q", result.ProtocolVersion, protocolVersion)
	}
	if result.ServerInfo.Name != "shellfish" {
		t.Errorf("serverInfo.name = %q, want %q", result.ServerInfo.Name, "shellfish")
	}
	if result.Capabilities.Tools == nil {
		t.Error("capabilities.tools should not be nil")
	}
}

func TestToolsList(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "tools/list", 2, nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	b, _ := json.Marshal(resp.Result)
	var result toolsListResult
	json.Unmarshal(b, &result)

	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "shell" {
		t.Errorf("tool name = %q, want %q", result.Tools[0].Name, "shell")
	}
	if result.Tools[0].InputSchema == nil {
		t.Error("inputSchema should not be nil")
	}

	desc := result.Tools[0].Description
	if !strings.Contains(desc, "/data") {
		t.Errorf("tool description should list /data mount, got: %s", desc)
	}
}

func TestToolsCallLs(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "tools/call", 3, map[string]any{
		"name":      "shell",
		"arguments": map[string]any{"command": "ls /data"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	b, _ := json.Marshal(resp.Result)
	var result toolsCallResult
	json.Unmarshal(b, &result)

	if len(result.Content) == 0 {
		t.Fatal("expected content blocks")
	}
	text := result.Content[0].Text
	if !strings.Contains(text, "hello.txt") {
		t.Errorf("ls output should contain hello.txt, got: %s", text)
	}
	if !strings.Contains(text, "subdir") {
		t.Errorf("ls output should contain subdir, got: %s", text)
	}
}

func TestToolsCallCat(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "tools/call", 4, map[string]any{
		"name":      "shell",
		"arguments": map[string]any{"command": "cat /data/hello.txt"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	b, _ := json.Marshal(resp.Result)
	var result toolsCallResult
	json.Unmarshal(b, &result)

	if result.Content[0].Text != "Hello, Shellfish!\n" {
		t.Errorf("cat output = %q, want %q", result.Content[0].Text, "Hello, Shellfish!\n")
	}
}

func TestToolsCallPipe(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "tools/call", 5, map[string]any{
		"name":      "shell",
		"arguments": map[string]any{"command": "echo hello world | cat"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}

	b, _ := json.Marshal(resp.Result)
	var result toolsCallResult
	json.Unmarshal(b, &result)

	if !strings.Contains(result.Content[0].Text, "hello world") {
		t.Errorf("pipe output should contain 'hello world', got: %s", result.Content[0].Text)
	}
}

func TestToolsCallCdPersistsState(t *testing.T) {
	srv := setupTestServer(t)

	// Send two commands: cd then pwd. Need to send them as a multi-line session.
	reqs := []struct {
		id      int
		command string
	}{
		{1, "cd /data"},
		{2, "pwd"},
	}

	var input bytes.Buffer
	for _, r := range reqs {
		req := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      mustJSON(r.id),
			Method:  "tools/call",
			Params:  mustJSON(map[string]any{"name": "shell", "arguments": map[string]any{"command": r.command}}),
		}
		line, _ := json.Marshal(req)
		input.Write(line)
		input.WriteByte('\n')
	}

	var out bytes.Buffer
	ctx := context.Background()
	if err := srv.Run(ctx, &input, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	dec := json.NewDecoder(&out)
	// Skip cd response
	var cdResp jsonRPCResponse
	if err := dec.Decode(&cdResp); err != nil {
		t.Fatalf("decode cd response: %v", err)
	}

	// Read pwd response
	var pwdResp jsonRPCResponse
	if err := dec.Decode(&pwdResp); err != nil {
		t.Fatalf("decode pwd response: %v", err)
	}

	b, _ := json.Marshal(pwdResp.Result)
	var result toolsCallResult
	json.Unmarshal(b, &result)

	if !strings.Contains(result.Content[0].Text, "/data") {
		t.Errorf("after cd /data, pwd should show /data, got: %s", result.Content[0].Text)
	}
}

func TestToolsCallUnknownTool(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "tools/call", 6, map[string]any{
		"name":      "nonexistent",
		"arguments": map[string]any{"command": "ls"},
	})

	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if resp.Error.Code != errCodeInvalidParams {
		t.Errorf("error code = %d, want %d", resp.Error.Code, errCodeInvalidParams)
	}
}

func TestToolsCallEmptyCommand(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "tools/call", 7, map[string]any{
		"name":      "shell",
		"arguments": map[string]any{"command": ""},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error.Message)
	}

	b, _ := json.Marshal(resp.Result)
	var result toolsCallResult
	json.Unmarshal(b, &result)

	if !result.IsError {
		t.Error("expected isError=true for empty command")
	}
}

func TestUnknownMethod(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "nonexistent/method", 8, nil)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != errCodeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, errCodeMethodNotFound)
	}
}

func TestPing(t *testing.T) {
	srv := setupTestServer(t)
	resp := roundTrip(t, srv, "ping", 9, nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Error("ping result should not be nil")
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
