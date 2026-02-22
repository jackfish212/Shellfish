// RSS Agent Example with httpfs
//
// This example demonstrates:
// 1. Using httpfs with RSSParser to monitor GitHub commit feeds
// 2. Building an AI agent that can read and analyze the RSS feed
// 3. Shell routing: "!xxx" -> shell, other -> LLM conversation
//
// The agent monitors the GRASP repository commits via Atom feed:
// https://github.com/jackfish212/GRASP/commits.atom
//
// Usage:
//
//	go run main.go
//
// Environment variables:
//
//	ANTHROPIC_API_KEY  - Required for LLM features (or use ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN)
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
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
	httpfs "github.com/jackfish212/grasp/httpfs"
	"github.com/joho/godotenv"
)

// Agent represents an AI agent with shell access
type Agent struct {
	client   anthropic.Client
	shell    *grasp.Shell
	vos      *grasp.VirtualOS
	messages []anthropic.MessageParam
}

// GitHubCommitsSource defines a GitHub commits Atom feed source
type GitHubCommitsSource struct {
	Name string // Display name for the source
	URL  string // Atom feed URL (e.g., https://github.com/user/repo/commits.atom)
}

func main() {
	// Flags
	interactive := flag.Bool("i", false, "Run in interactive mode")
	flag.Parse()

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
		os.Exit(1)
	}

	// Setup VirtualOS with httpfs
	v, fs := setupVirtualOS()

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

	if *interactive {
		// Run interactive loop
		agent.Run(context.Background())
	} else {
		// Run demo mode
		runDemo(context.Background(), agent, fs)
	}
}

func setupVirtualOS() (*grasp.VirtualOS, *httpfs.HTTPFS) {
	v := grasp.New()

	// Configure standard filesystem layout
	rootFS, err := grasp.Configure(v)
	if err != nil {
		panic(err)
	}

	// Register built-in commands
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// === HTTPFS with RSS Parser ===
	// Monitor GitHub commits for the GRASP repository
	fs := httpfs.NewHTTPFS(
		httpfs.WithHTTPFSInterval(5*time.Minute), // Poll every 5 minutes
	)

	// Add GitHub commits Atom feeds
	// Using RSSParser which supports both RSS 2.0 and Atom feeds
	// Note: GitHub requires network access. If unavailable, the demo will show empty feed.
	sources := []GitHubCommitsSource{
		{
			Name: "grasp-commits",
			URL:  "https://github.com/jackfish212/GRASP/commits/main.atom",
		},
		// You can add more repositories to monitor:
		// {
		// 	Name: "httpfs-commits",
		// 	URL:  "https://github.com/jackfish212/grasp/httpfs/commits/main.atom",
		// },
	}

	for _, src := range sources {
		if err := fs.Add(src.Name, src.URL, &httpfs.RSSParser{}); err != nil {
			fmt.Printf("Warning: Could not add source %s: %v\n", src.Name, err)
		} else {
			fmt.Printf("Added RSS source: %s -> %s\n", src.Name, src.URL)
		}
	}

	// Mount httpfs
	v.Mount("/feeds", fs)

	// Start polling
	fs.Start(context.Background())

	return v, fs
}

func (a *Agent) Run(ctx context.Context) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║            RSS Agent - httpfs Demo                            ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Commands:                                                    ║")
	fmt.Println("║    !<cmd>  - Execute shell command directly                   ║")
	fmt.Println("║    <text>  - Chat with AI agent                              ║")
	fmt.Println("║    exit    - Quit                                            ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Mount Points:                                                ║")
	fmt.Println("║    /feeds/grasp-commits - GRASP repository commits (Atom)    ║")
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
RSS Agent Help
==================

Shell Commands (prefix with !):
  !ls /feeds                    - List all RSS sources
  !ls /feeds/grasp-commits      - List recent commits
  !cat /feeds/grasp-commits/<file> - Read a specific commit
  !grep "fix" /feeds/grasp-commits - Search for commits with "fix"
  !mount                        - Show all mount points

LLM Conversation:
  Any text without ! prefix will be sent to the AI agent.
  The agent can use shell commands to help you analyze the feed.

Examples:
  > What are the latest commits?
  > Summarize recent changes
  > Find commits related to bug fixes
  > When was the last commit?

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
		Description: anthropic.String("Execute a shell command in the virtual filesystem. Use this to explore RSS feeds, read commit messages, or search for patterns."),
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
	systemPrompt := `You are an AI agent with access to RSS/Atom feeds through httpfs.

The filesystem is mounted at /feeds and contains GitHub commit feeds:
- /feeds/grasp-commits - GRASP repository commits (Atom feed)

Each commit is stored as a separate .txt file with:
- Title: Commit message
- Link: GitHub commit URL
- Date: Commit timestamp
- Description: Commit details

Shell commands available:
- ls /feeds/grasp-commits      - List recent commits
- cat /feeds/grasp-commits/<file>.txt - Read a specific commit
- grep "pattern" /feeds/grasp-commits - Search for commits
- head -5 /feeds/grasp-commits/<file>.txt - Preview commit

Guidelines:
1. Explore the feed structure first using ls
2. Read commit files to understand recent changes
3. Be concise in your responses
4. Summarize information rather than showing raw output`

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

// runDemo runs a non-interactive demo showing the httpfs RSS capabilities
func runDemo(ctx context.Context, agent *Agent, fs *httpfs.HTTPFS) {
	defer fs.Stop()

	fmt.Println("=== RSS Agent Demo (non-interactive mode) ===")
	fmt.Println()

	// Wait for initial fetch to complete (fetchAll is sync in Start, but give a moment)
	fmt.Println("Waiting for RSS feed to be fetched...")
	time.Sleep(2 * time.Second)

	// Show sources info
	fmt.Println("\n--- Demo 1: RSS sources info ---")
	sources := fs.Sources()
	for name, url := range sources {
		fmt.Printf("  %s -> %s\n", name, url)
	}

	// Demo 2: List RSS sources via shell
	fmt.Println("\n--- Demo 2: List RSS sources via shell ---")
	result := agent.shell.Execute(ctx, "ls /feeds")
	fmt.Print(result.Output)

	// Demo 3: List commits in the feed
	fmt.Println("\n--- Demo 3: List recent commits ---")
	result = agent.shell.Execute(ctx, "ls /feeds/grasp-commits")
	fmt.Printf("  (output: %q)\n", strings.TrimSpace(result.Output))

	// Check if we have files
	files := strings.Fields(result.Output)
	if len(files) == 0 || (len(files) == 1 && strings.HasSuffix(files[0], "/")) {
		fmt.Println("  No commit files found. The feed might be empty or fetch failed.")

		// Try a simple query anyway
		fmt.Println("\n--- Demo 4: Ask AI agent (feed might be empty) ---")
		agent.chatWithLLM(ctx, "Check if there are any commits available. If not, explain why.")
	} else {
		// Demo 4: Read the first commit
		fmt.Println("\n--- Demo 4: Read the latest commit ---")
		// Get the first file (remove trailing / if present)
		firstFile := strings.TrimSuffix(files[0], "/")
		if firstFile != "" {
			result = agent.shell.Execute(ctx, "cat /feeds/grasp-commits/"+firstFile)
			fmt.Print(result.Output)
		}

		// Demo 5: Ask the AI agent about the commits
		fmt.Println("\n--- Demo 5: Ask AI agent about recent commits ---")
		agent.chatWithLLM(ctx, "What are the latest 3 commits? Briefly summarize each one.")
	}

	fmt.Println("\n=== Demo complete ===")
	fmt.Println("Run with -i flag for interactive mode")
}
