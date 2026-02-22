// Example: GitHub MCP Server -- Connect via Streamable HTTP transport
//
// Demonstrates connecting to GitHub's official MCP Server
// (https://github.com/github/github-mcp-server) using HttpMCPClient
// and mounting its tools into a grasp VOS namespace.
//
// Run:
//
//	cd examples/github-mcp
//	cp .env.example .env   # fill in credentials
//	go run .
//
// Flags:
//
//	-i    Run in interactive mode
//	-v    Show each shell command and its output
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mounts"
	"github.com/joho/godotenv"
)

var verbose bool

func main() {
	interactive := flag.Bool("i", false, "Run in interactive mode")
	flag.BoolVar(&verbose, "v", false, "Show shell commands and output")
	flag.Parse()

	if err := godotenv.Load(filepath.Join(".", ".env")); err != nil {
		log.Printf("Note: No .env file found, using environment variables")
	}

	ctx := context.Background()

	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		log.Fatalf("Configure VOS: %v", err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	mcpURL := os.Getenv("GITHUB_MCP_URL")
	if mcpURL == "" {
		mcpURL = "https://api.githubcopilot.com/mcp/"
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	mcpClient := mounts.NewHttpMCPClient(mcpURL, mounts.WithBearerToken(token))

	fmt.Printf("Connecting to GitHub MCP Server at %s ...\n", mcpURL)
	serverInfo, err := mcpClient.Initialize(ctx)
	if err != nil {
		log.Fatalf("MCP initialize failed: %v", err)
	}
	if si, ok := serverInfo["serverInfo"].(map[string]any); ok {
		fmt.Printf("Connected to: %v %v\n", si["name"], si["version"])
	}

	if err := mounts.MountMCP(v, "/github", mcpClient); err != nil {
		log.Fatalf("Mount GitHub MCP: %v", err)
	}

	workFS := mounts.NewMemFS(grasp.PermRW)
	if err := v.Mount("/workspace", workFS); err != nil {
		log.Fatalf("Mount workspace: %v", err)
	}

	tools, err := mcpClient.ListTools(ctx)
	if err != nil {
		log.Printf("Warning: Could not list tools: %v", err)
	} else {
		fmt.Printf("\nGitHub MCP tools available: %d\n", len(tools))
		shown := 5
		if verbose {
			shown = len(tools)
		}
		if shown > len(tools) {
			shown = len(tools)
		}
		for _, t := range tools[:shown] {
			fmt.Printf("  %s - %s\n", t.Name, truncate(t.Description, 60))
		}
		if len(tools) > shown {
			fmt.Printf("  ... and %d more (use -v to see all)\n", len(tools)-shown)
		}
	}

	client := newAnthropicClient()
	shellTool := shellToolDef()

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  grasp + GitHub MCP Server (HTTP)")
	fmt.Println("========================================")
	fmt.Println()

	if *interactive {
		runInteractiveMode(ctx, v, client, shellTool)
	} else {
		runDemoMode(ctx, v, client, shellTool)
	}
}

func runInteractiveMode(ctx context.Context, v *grasp.VirtualOS, client anthropic.Client, tool anthropic.ToolParam) {
	fmt.Println("Interactive Mode (type 'exit' to quit)")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	messages := []anthropic.MessageParam{}
	system := `You are an AI assistant with access to GitHub through the MCP protocol.

Available shell commands:
- ls /github/tools/ to list all GitHub MCP tools
- /github/tools/<tool-name> --param value to execute a GitHub tool
- cat, grep, echo, write for standard filesystem commands
- /workspace/ is a writable area for notes and output

Common GitHub tools:
- search-repositories --query "..." to search GitHub repos
- get-file-contents --owner X --repo Y --path Z to read a file
- search-code --query "..." to search code across GitHub
- list-issues --owner X --repo Y to list issues
- issue-read --method get --owner X --repo Y --issue-number N to read an issue`

	for {
		fmt.Print("You: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if strings.ToLower(input) == "exit" || strings.ToLower(input) == "quit" {
			fmt.Println("Goodbye!")
			break
		}
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(input)))
		messages = agentLoop(ctx, v, client, tool, messages, system)
	}
}

func runDemoMode(ctx context.Context, v *grasp.VirtualOS, client anthropic.Client, tool anthropic.ToolParam) {
	fmt.Println("Demo Mode")
	fmt.Println("=========")
	fmt.Println()

	task := `You have access to GitHub via MCP tools mounted at /github/tools/.

Please complete these tasks:

1. First, list available tools with: ls /github/tools/
2. Search for repositories related to "agent filesystem" using the search-repositories tool
3. Pick the most interesting result and get its README using get-file-contents
4. Write a brief summary of what you found to /workspace/report.md`

	fmt.Println("Task: Explore GitHub via MCP tools")
	fmt.Println()

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(task)),
	}
	system := "You are an AI assistant exploring GitHub through MCP tools mounted in a virtual filesystem. Use shell commands to interact with /github/tools/ and write results to /workspace/."
	agentLoop(ctx, v, client, tool, messages, system)

	fmt.Println()
	fmt.Println("========== REPORT ==========")
	sh := v.Shell("viewer")
	result := sh.Execute(ctx, "cat /workspace/report.md")
	if result.Output != "" {
		fmt.Println(result.Output)
	} else {
		fmt.Println("(no report generated)")
	}
	fmt.Println("\n[Demo Complete]")
}

func agentLoop(ctx context.Context, v *grasp.VirtualOS, client anthropic.Client, tool anthropic.ToolParam, messages []anthropic.MessageParam, system string) []anthropic.MessageParam {
	sh := v.Shell("agent")
	model := getModel()

	for {
		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: 4096,
			System:    []anthropic.TextBlockParam{{Text: system}},
			Messages:  messages,
			Tools:     []anthropic.ToolUnionParam{{OfTool: &tool}},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "API error: %v\n", err)
			return messages
		}

		messages = append(messages, resp.ToParam())

		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				fmt.Printf("\nAssistant: %s\n", b.Text)
			case anthropic.ToolUseBlock:
				hasToolUse = true
				var input struct {
					Command string `json:"command"`
				}
				json.Unmarshal([]byte(b.JSON.Input.Raw()), &input)

				if verbose {
					fmt.Printf("  $ %s\n", input.Command)
				} else {
					fmt.Print(".")
				}

				result := sh.Execute(ctx, input.Command)
				output := result.Output
				if result.Code != 0 {
					if output != "" && !strings.HasSuffix(output, "\n") {
						output += "\n"
					}
					output += fmt.Sprintf("[exit code: %d]", result.Code)
				}

				if verbose && output != "" {
					fmt.Printf("  %s\n", truncate(output, 500))
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, output, result.Code != 0))
			}
		}

		if !hasToolUse {
			break
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	return messages
}

func shellToolDef() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name:        "shell",
		Description: anthropic.String("Execute a command in the grasp virtual filesystem. GitHub MCP tools are at /github/tools/. Use ls to discover tools, then execute them with --param flags. Standard commands (cat, grep, echo, write) also available. Supports pipes (|) and redirects (>, >>)."),
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
}

func getModel() anthropic.Model {
	if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
		return anthropic.Model(m)
	}
	return anthropic.ModelClaudeSonnet4_5_20250929
}

func newAnthropicClient() anthropic.Client {
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if baseURL != "" && authToken != "" {
		return anthropic.NewClient(
			option.WithBaseURL(baseURL),
			option.WithAPIKey(authToken),
		)
	}
	return anthropic.NewClient()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "... (truncated)"
}
