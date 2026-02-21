package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/shell"
)

// Server implements the MCP protocol over stdio, exposing a grasp VirtualOS
// as a single "shell" tool. Shell state (cwd, env, history) persists across
// tool calls within the same session.
type Server struct {
	vos   *grasp.VirtualOS
	shell *shell.Shell
	info  grasp.VersionInfo
}

// New creates an MCP server bound to the given VirtualOS.
// The user parameter sets the shell's $USER and determines $HOME.
func New(vos *grasp.VirtualOS, user string) *Server {
	return &Server{
		vos:   vos,
		shell: vos.Shell(user),
		info:  grasp.GetVersionInfo(),
	}
}

// Run starts the MCP server, reading JSON-RPC messages from in and writing
// responses to out. It blocks until in is closed or ctx is cancelled.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	enc := json.NewEncoder(out)

	slog.Info("grasp-server started", "version", s.info.Version)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			slog.Info("grasp-server: context cancelled")
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			slog.Warn("invalid JSON-RPC message", "error", err)
			resp := &jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &jsonRPCError{Code: errCodeParse, Message: "Parse error"},
			}
			if err := enc.Encode(resp); err != nil {
				return fmt.Errorf("write error: %w", err)
			}
			continue
		}

		resp := s.dispatch(ctx, &req)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("write error: %w", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read error: %w", err)
	}

	slog.Info("grasp-server: stdin closed, shutting down")
	return nil
}

func (s *Server) dispatch(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized", "initialized":
		return nil
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	case "ping":
		return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	default:
		slog.Debug("unknown method", "method", req.Method)
		if req.ID != nil {
			return &jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonRPCError{Code: errCodeMethodNotFound, Message: "Method not found: " + req.Method},
			}
		}
		return nil
	}
}

// ─── Handlers ───

func (s *Server) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	var params initializeParams
	if req.Params != nil {
		json.Unmarshal(req.Params, &params)
	}
	slog.Info("client connected",
		"client", params.ClientInfo.Name,
		"clientVersion", params.ClientInfo.Version,
		"protocolVersion", params.ProtocolVersion,
	)

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: initializeResult{
			ProtocolVersion: protocolVersion,
			Capabilities:    serverCapabilities{Tools: &toolsCapability{}},
			ServerInfo:      serverInfo{Name: "grasp", Version: s.info.Version},
		},
	}
}

func (s *Server) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	desc := s.buildToolDescription()

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: toolsListResult{
			Tools: []toolDef{{
				Name:        "shell",
				Description: desc,
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{
							"type":        "string",
							"description": "The shell command to execute",
						},
					},
					"required": []string{"command"},
				},
			}},
		},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: errCodeInvalidParams, Message: "Invalid params: " + err.Error()},
		}
	}

	if params.Name != "shell" {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: errCodeInvalidParams, Message: "Unknown tool: " + params.Name},
		}
	}

	command, _ := params.Arguments["command"].(string)
	if command == "" {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  toolsCallResult{Content: []contentBlock{{Type: "text", Text: "error: command is required"}}, IsError: true},
		}
	}

	slog.Debug("executing", "command", command)
	result := s.shell.Execute(ctx, command)

	output := result.Output
	if result.Code != 0 {
		if output != "" && !strings.HasSuffix(output, "\n") {
			output += "\n"
		}
		output += fmt.Sprintf("[exit code: %d]", result.Code)
	}

	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  toolsCallResult{Content: []contentBlock{{Type: "text", Text: output}}},
	}
}

// ─── Helpers ───

func (s *Server) buildToolDescription() string {
	var b strings.Builder
	b.WriteString("Execute a shell command in the grasp virtual filesystem. ")
	b.WriteString("Commands: ls, cat, read, write, stat, grep, find, head, tail, mkdir, rm, mv, cp, mount, which, uname. ")
	b.WriteString("Shell builtins: cd, pwd, echo, env, history. ")
	b.WriteString("Features: pipes (|), redirects (>, >>), logical operators (&&, ||), here-documents (<<EOF), env vars ($VAR).")

	mounts := s.vos.MountTable().All()
	if len(mounts) > 0 {
		b.WriteString("\n\nMounted filesystems:")
		for _, mp := range mounts {
			b.WriteString("\n  " + mp)
		}
	}

	return b.String()
}
