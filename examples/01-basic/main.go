// Full Demo: All Provider Types with Anthropic Agent and Shell Routing
//
// This example demonstrates:
// 1. Mounting all available provider types
// 2. Shell routing: "!xxx" → shell, other → LLM conversation
// 3. Integration with Anthropic Claude API
//
// Usage:
//
//	go run main.go
//
// Environment variables:
//
//	ANTHROPIC_API_KEY  - Required for LLM features (or use ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN)
//	GITHUB_TOKEN       - Optional, for GitHub provider
//	MCP_FILESYSTEM_CMD - Optional, e.g., "npx -y @modelcontextprotocol/server-filesystem /path"
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mounts"
	"github.com/joho/godotenv"
)

type Agent struct {
	client   anthropic.Client
	shell    *grasp.Shell
	vos      *grasp.VirtualOS
	messages []anthropic.MessageParam
}

func main() {
	// Load .env file
	envPath := filepath.Join(".", ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Check for API key
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")

	if apiKey == "" && authToken == "" {
		fmt.Println("Error: ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN is required")
		fmt.Println("\nUsage:")
		fmt.Println("  ANTHROPIC_API_KEY=sk-xxx go run main.go")
		fmt.Println("\nOr for custom endpoints:")
		fmt.Println("  ANTHROPIC_BASE_URL=https://api.example.com ANTHROPIC_AUTH_TOKEN=xxx go run main.go")
		fmt.Println("\nOptional:")
		fmt.Println("  GITHUB_TOKEN=ghp_xxx       - Enable GitHub provider")
		fmt.Println("  MCP_FILESYSTEM_CMD='npx...' - Enable MCP filesystem tools")
		os.Exit(1)
	}

	// Setup VirtualOS with all providers
	v := setupVirtualOS()

	// Create client
	var client anthropic.Client
	if baseURL != "" && authToken != "" {
		client = anthropic.NewClient(
			option.WithBaseURL(baseURL),
			option.WithAPIKey(authToken),
		)
	} else {
		client = anthropic.NewClient()
	}

	// Create agent
	agent := &Agent{
		client: client,
		shell:  v.Shell("user"),
		vos:    v,
	}

	// Run interactive loop
	agent.Run(context.Background())
}

func setupVirtualOS() *grasp.VirtualOS {
	v := grasp.New()

	// Configure standard filesystem layout
	rootFS, err := grasp.Configure(v)
	if err != nil {
		panic(err)
	}

	// Register built-in commands
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// === Local filesystem ===
	cwd, _ := os.Getwd()
	v.Mount("/data", mounts.NewLocalFS(cwd, grasp.PermRW))

	// === In-memory filesystem ===
	memFS := mounts.NewMemFS(grasp.PermRW)
	memFS.AddFile("readme.txt", []byte("# Welcome to grasp Agent\n\nThis is the in-memory workspace.\n"), grasp.PermRO)
	v.Mount("/memory", memFS)

	// === GitHub API ===
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		githubFS := mounts.NewGitHubFS(
			mounts.WithGitHubToken(token),
			mounts.WithGitHubCacheTTL(5*time.Minute),
		)
		v.Mount("/github", githubFS)
		fmt.Println("GitHub provider mounted at /github")
	}

	// === MCP servers ===
	if mcpCmd := os.Getenv("MCP_FILESYSTEM_CMD"); mcpCmd != "" {
		// Note: NewStdioMCPClient requires stdin/stdout for the subprocess
		// In a real app, you would handle this differently
		fmt.Printf("MCP command configured: %s\n", mcpCmd)
	}

	return v
}

func (a *Agent) Run(ctx context.Context) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║          grasp Agent - Full Demo                         ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Commands:                                                    ║")
	fmt.Println("║    !<cmd>  - Execute shell command directly                   ║")
	fmt.Println("║    <text>  - Chat with AI agent                              ║")
	fmt.Println("║    exit    - Quit                                            ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Mount Points:                                                ║")
	fmt.Println("║    /data     - Local filesystem (read/write)                 ║")
	fmt.Println("║    /memory   - In-memory workspace (read/write)              ║")

	if _, err := a.vos.Stat(ctx, "/github"); err == nil {
		fmt.Println("║    /github   - GitHub API (read-only)                        ║")
	}
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		switch strings.ToLower(input) {
		case "exit", "quit", "q":
			fmt.Println("Goodbye!")
			return
		case "help":
			a.showHelp()
			continue
		case "mounts":
			result := a.shell.Execute(ctx, "mount")
			fmt.Println(result.Output)
			continue
		}

		// Route command
		if strings.HasPrefix(input, "!") {
			// Direct shell execution
			cmd := strings.TrimPrefix(input, "!")
			a.executeShell(ctx, cmd)
		} else {
			// LLM conversation
			a.chatWithLLM(ctx, input)
		}
	}
}

func (a *Agent) showHelp() {
	fmt.Println(`
grasp Agent Help
====================

Shell Commands (prefix with !):
  !ls /data           - List files in local directory
  !cat /memory/readme.txt - Read in-memory file
  !grep pattern /data/*.go - Search for pattern
  !search "query" --scope /data - Full-text search
  !mount              - Show all mount points

LLM Conversation:
  Any text without ! prefix will be sent to the AI agent.
  The agent can use shell commands to help you.

Examples:
  > List all Go files in /data
  > What's in the readme?
  > Search for TODO comments

Special Commands:
  help    - Show this help
  mounts  - Show mount points
  exit    - Quit the agent
`)
}

func (a *Agent) executeShell(ctx context.Context, cmd string) {
	result := a.shell.Execute(ctx, cmd)
	if result.Code != 0 {
		fmt.Printf("Error (exit %d): %s", result.Code, result.Output)
	} else if result.Output == "" {
		// No output - show a hint for common commands
		if cmd == "ls" || strings.HasPrefix(cmd, "ls ") {
			fmt.Println("(empty directory)")
		}
	} else {
		fmt.Print(result.Output)
	}
}

func (a *Agent) chatWithLLM(ctx context.Context, userMessage string) {
	// Add user message to history
	a.messages = append(a.messages, anthropic.NewUserMessage(
		anthropic.NewTextBlock(userMessage),
	))

	// Define tools
	shellTool := anthropic.ToolParam{
		Name:        "shell",
		Description: anthropic.String("Execute a shell command in the virtual filesystem. Supports pipes, redirects, and composition. Use this to explore files, search content, or run any shell command."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "Shell command to execute",
				},
			},
			Required: []string{"command"},
		},
	}

	readTool := anthropic.ToolParam{
		Name:        "read",
		Description: anthropic.String("Read a file from the virtual filesystem. Returns the file content."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
			Properties: map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "File path to read",
				},
			},
			Required: []string{"path"},
		},
	}

	// System prompt
	systemPrompt := `You are an AI agent with access to a virtual filesystem through grasp.

Available mount points:
- /data - Local project files (read/write)
- /memory - In-memory workspace (read/write)
- /github - GitHub API (if configured, read-only)

Shell commands available:
- Navigation: ls, cd, pwd, find, stat
- File operations: cat, read, write, mkdir, rm, mv
- Search: grep, search
- System: mount, which, uname

Use pipes and composition for efficient operations:
  cat /data/log.md | grep error | head -5
  search "TODO" --scope /data

Guidelines:
1. Explore before modifying: use ls, cat, stat to understand structure
2. Use /memory for temporary outputs and experiments
3. Be concise in your responses
4. When asked to analyze code or files, read them first with shell commands`

	// Call LLM in a loop to handle tool calls
	for {
		message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_5_20250929,
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Tools:    []anthropic.ToolUnionParam{{OfTool: &shellTool}, {OfTool: &readTool}},
			Messages: a.messages,
		})

		if err != nil {
			fmt.Printf("Error calling LLM: %v\n", err)
			return
		}

		// Add assistant message to history
		a.messages = append(a.messages, message.ToParam())

		// Check for tool calls
		var toolResults []anthropic.ContentBlockParamUnion
		var hasToolCalls bool
		var textResponse string

		for _, block := range message.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				textResponse += b.Text
			case anthropic.ToolUseBlock:
				hasToolCalls = true
				result := a.executeTool(ctx, b)
				toolResults = append(toolResults, result)
			}
		}

		if !hasToolCalls {
			// No more tool calls - show response
			fmt.Println(textResponse)
			return
		}

		// Add tool results and continue
		a.messages = append(a.messages, anthropic.NewUserMessage(toolResults...))
	}
}

func (a *Agent) executeTool(ctx context.Context, block anthropic.ToolUseBlock) anthropic.ContentBlockParamUnion {
	var output string
	isError := false

	switch block.Name {
	case "shell":
		var input struct {
			Command string `json:"command"`
		}
		inputJSON := block.JSON.Input.Raw()
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			output = fmt.Sprintf("Parse error: %v", err)
			isError = true
		} else {
			fmt.Printf("[shell] %s\n", input.Command)
			result := a.shell.Execute(ctx, input.Command)
			output = result.Output
			if result.Code != 0 {
				isError = true
				if output == "" {
					output = fmt.Sprintf("Command exited with code %d", result.Code)
				}
			}
			// Truncate large outputs
			if len(output) > 50000 {
				output = output[:50000] + "\n... (output truncated)"
			}
			fmt.Printf("[result] %s\n", truncate(output, 200))
		}

	case "read":
		var input struct {
			Path string `json:"path"`
		}
		inputJSON := block.JSON.Input.Raw()
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			output = fmt.Sprintf("Parse error: %v", err)
			isError = true
		} else {
			file, err := a.vos.Open(ctx, input.Path)
			if err != nil {
				output = fmt.Sprintf("Read error: %v", err)
				isError = true
			} else {
				defer file.Close()
				data, err := io.ReadAll(file)
				if err != nil {
					output = fmt.Sprintf("Read error: %v", err)
					isError = true
				} else {
					output = string(data)
					if len(output) > 50000 {
						output = output[:50000] + "\n... (output truncated)"
					}
				}
			}
		}

	default:
		output = fmt.Sprintf("Unknown tool: %s", block.Name)
		isError = true
	}

	return anthropic.NewToolResultBlock(block.ID, output, isError)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
