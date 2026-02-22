// Dynamic Mount Agent Example with httpfs
//
// This example demonstrates:
// 1. An AI agent that can dynamically add/remove HTTP mounts at runtime
// 2. Using shell commands to manage httpfs sources
// 3. Multiple parser types (RSS, JSON, Raw)
//
// The agent can:
// - Add new RSS/JSON/Raw sources via shell commands
// - List current mounts and sources
// - Remove sources that are no longer needed
//
// Usage:
//
//	go run main.go [-i]
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

// DynamicAgent represents an AI agent that can manage HTTP mounts dynamically
type DynamicAgent struct {
	client   anthropic.Client
	shell    *grasp.Shell
	vos      *grasp.VirtualOS
	fs       *httpfs.HTTPFS
	messages []anthropic.MessageParam
}

// AddSourceRequest represents a request to add a new HTTP source
type AddSourceRequest struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	ParserType  string            `json:"parserType"` // "rss", "json", "raw"
	Headers     map[string]string `json:"headers,omitempty"`
	// JSON parser options
	NameField  string `json:"nameField,omitempty"`
	IDField    string `json:"idField,omitempty"`
	ArrayField string `json:"arrayField,omitempty"`
	// Raw parser options
	Filename string `json:"filename,omitempty"`
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
	agent := &DynamicAgent{
		client: client,
		shell:  v.Shell("user"),
		vos:    v,
		fs:     fs,
	}

	if *interactive {
		// Run interactive loop
		agent.Run(context.Background())
	} else {
		// Run demo mode
		runDemo(context.Background(), agent)
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

	// Create HTTPFS with short interval for demo
	fs := httpfs.NewHTTPFS(
		httpfs.WithHTTPFSInterval(30*time.Second), // Poll every 30 seconds
	)

	// Mount httpfs at /http
	v.Mount("/http", fs)

	// Start polling
	fs.Start(context.Background())

	return v, fs
}

func (a *DynamicAgent) Run(ctx context.Context) {
	reader := bufio.NewReader(os.Stdin)

	a.printWelcome()

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
			a.fs.Stop()
			return
		case "help":
			a.showHelp()
			continue
		case "mounts":
			result := a.shell.Execute(ctx, "mount")
			fmt.Println(result.Output)
			continue
		case "sources":
			a.listSources()
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

func (a *DynamicAgent) printWelcome() {
	fmt.Println("╔═══════════════════════════════════════════════════════════════╗")
	fmt.Println("║         Dynamic Mount Agent - httpfs Demo                     ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  This agent can dynamically add/remove HTTP mounts!          ║")
	fmt.Println("║                                                               ║")
	fmt.Println("║  Commands:                                                    ║")
	fmt.Println("║    !<cmd>  - Execute shell command directly                   ║")
	fmt.Println("║    <text>  - Chat with AI (can add mounts for you)           ║")
	fmt.Println("║    sources - List all HTTP sources                           ║")
	fmt.Println("║    mounts  - Show mount points                               ║")
	fmt.Println("║    help    - Show detailed help                              ║")
	fmt.Println("║    exit    - Quit                                            ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func (a *DynamicAgent) showHelp() {
	fmt.Println(`
Dynamic Mount Agent Help
========================

Shell Commands (prefix with !):
  !ls /http                    - List all HTTP sources
  !ls /http/<source>           - List files in a source
  !cat /http/<source>/<file>   - Read a specific file
  !mount                       - Show all mount points

AI Capabilities:
  Ask the AI to help you manage HTTP sources! It can:
  - Add RSS/Atom feeds
  - Add JSON API endpoints
  - Add raw text/HTML endpoints
  - Remove sources you no longer need

Examples (talk to AI):
  > Add the Hacker News RSS feed
  > Monitor this JSON API: https://api.example.com/data
  > Add a GitHub commits feed for user/repo
  > Remove the hacker-news source
  > What sources are currently mounted?

Direct Add Commands (via shell):
  !httpfs add rss hacker-news https://hnrss.org/frontpage
  !httpfs add json users https://api.example.com/users
  !httpfs remove hacker-news

Parser Types:
  - rss  : RSS 2.0 and Atom feeds
  - json : JSON arrays or nested data
  - raw  : Plain text/HTML as single file

Special Commands:
  help    - Show this help
  sources - List HTTP sources
  mounts  - Show mount points
  exit    - Quit the agent
`)
}

func (a *DynamicAgent) listSources() {
	sources := a.fs.Sources()
	if len(sources) == 0 {
		fmt.Println("No HTTP sources configured.")
		fmt.Println("Ask the AI to add one, e.g.: 'Add the GitHub blog RSS feed'")
		return
	}
	fmt.Println("HTTP Sources:")
	for name, url := range sources {
		fmt.Printf("  %-20s -> %s\n", name, url)
	}
}

func (a *DynamicAgent) executeShell(ctx context.Context, cmd string) {
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

func (a *DynamicAgent) addSource(ctx context.Context, req AddSourceRequest) (string, error) {
	var parser httpfs.ResponseParser

	switch req.ParserType {
	case "rss", "atom", "":
		parser = &httpfs.RSSParser{}
	case "json":
		parser = &httpfs.JSONParser{
			NameField:  req.NameField,
			IDField:    req.IDField,
			ArrayField: req.ArrayField,
		}
	case "raw":
		parser = &httpfs.RawParser{
			Filename: req.Filename,
		}
		if parser.(*httpfs.RawParser).Filename == "" {
			parser.(*httpfs.RawParser).Filename = "content.txt"
		}
	default:
		return "", fmt.Errorf("unknown parser type: %s (use: rss, json, raw)", req.ParserType)
	}

	// Build source options
	var opts []httpfs.SourceOption
	for key, value := range req.Headers {
		opts = append(opts, httpfs.WithSourceHeader(key, value))
	}

	// Add the source
	if err := a.fs.Add(req.Name, req.URL, parser, opts...); err != nil {
		return "", err
	}

	return fmt.Sprintf("Added HTTP source '%s' -> %s (parser: %s)", req.Name, req.URL, req.ParserType), nil
}

func (a *DynamicAgent) removeSource(name string) (string, error) {
	if err := a.fs.RemoveSource(name); err != nil {
		return "", err
	}
	return fmt.Sprintf("Removed HTTP source '%s'", name), nil
}

func (a *DynamicAgent) chatWithLLM(ctx context.Context, userMessage string) {
	// Add user message to history
	a.messages = append(a.messages, anthropic.NewUserMessage(
		anthropic.NewTextBlock(userMessage),
	))

	// Define tools
	tools := []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        "shell",
				Description: anthropic.String("Execute a shell command in the virtual filesystem. Use ls to explore, cat to read files."),
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
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "add_http_source",
				Description: anthropic.String("Add a new HTTP source to monitor. The agent can dynamically mount RSS feeds, JSON APIs, or raw endpoints."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: constant.ValueOf[constant.Object](),
					Properties: map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Unique name for this source (will be used as directory name)",
						},
						"url": map[string]interface{}{
							"type":        "string",
							"description": "HTTP URL to fetch",
						},
						"parserType": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"rss", "json", "raw"},
							"description": "Parser type: rss for RSS/Atom feeds, json for JSON APIs, raw for plain text/HTML",
						},
						"headers": map[string]interface{}{
							"type":        "object",
							"description": "Optional HTTP headers (e.g., Authorization)",
							"additionalProperties": map[string]interface{}{
								"type": "string",
							},
						},
						"nameField": map[string]interface{}{
							"type":        "string",
							"description": "For JSON parser: field to use as file name",
						},
						"idField": map[string]interface{}{
							"type":        "string",
							"description": "For JSON parser: field to use as unique ID",
						},
						"arrayField": map[string]interface{}{
							"type":        "string",
							"description": "For JSON parser: dot-notation path to array (e.g., 'data.items')",
						},
						"filename": map[string]interface{}{
							"type":        "string",
							"description": "For raw parser: filename for the content (default: content.txt)",
						},
					},
					Required: []string{"name", "url", "parserType"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "remove_http_source",
				Description: anthropic.String("Remove an HTTP source that was previously added."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: constant.ValueOf[constant.Object](),
					Properties: map[string]interface{}{
						"name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the source to remove",
						},
					},
					Required: []string{"name"},
				},
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "list_http_sources",
				Description: anthropic.String("List all currently configured HTTP sources."),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type: constant.ValueOf[constant.Object](),
					Properties: map[string]interface{}{},
				},
			},
		},
	}

	// System prompt
	systemPrompt := `You are an AI agent with dynamic HTTP filesystem management capabilities.

The httpfs is mounted at /http and can dynamically add/remove HTTP sources.

CURRENT SOURCES:
The user may ask you to add, remove, or explore HTTP sources. Common use cases:

**RSS/Atom Feeds** (parserType: "rss"):
- GitHub commits: https://github.com/{user}/{repo}/commits/{branch}.atom
- Hacker News: https://hnrss.org/frontpage
- Any RSS 2.0 or Atom feed

**JSON APIs** (parserType: "json"):
- REST APIs returning JSON arrays
- Use nameField/idField/arrayField to configure parsing

**Raw Content** (parserType: "raw"):
- Plain text files
- HTML pages (single file)
- Any HTTP endpoint returning text

When the user asks to:
1. "Add [feed/API]" -> Use add_http_source tool
2. "Monitor [URL]" -> Use add_http_source tool
3. "Remove [source]" -> Use remove_http_source tool
4. "What sources?" -> Use list_http_sources tool
5. "Show me [content]" -> Use shell tool to ls/cat

Guidelines:
- Choose sensible source names (lowercase, hyphens, no spaces)
- Default to "rss" parser for unknown feed types
- After adding a source, wait a moment then offer to explore it
- Be helpful and proactive in suggesting sources`

	// Call LLM in a loop to handle tool calls
	for {
		message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_5_20250929,
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Tools:    tools,
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

func (a *DynamicAgent) executeTool(ctx context.Context, block anthropic.ToolUseBlock) anthropic.ContentBlockParamUnion {
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
			if len(output) > 50000 {
				output = output[:50000] + "\n... (output truncated)"
			}
			fmt.Printf("[result] %s\n", truncate(output, 200))
		}

	case "add_http_source":
		var input AddSourceRequest
		inputJSON := block.JSON.Input.Raw()
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			output = fmt.Sprintf("Parse error: %v", err)
			isError = true
		} else {
			fmt.Printf("[add_http_source] name=%s url=%s parser=%s\n", input.Name, input.URL, input.ParserType)
			result, err := a.addSource(ctx, input)
			if err != nil {
				output = fmt.Sprintf("Failed to add source: %v", err)
				isError = true
			} else {
				output = result
				// Trigger immediate fetch
				time.Sleep(500 * time.Millisecond)
			}
		}

	case "remove_http_source":
		var input struct {
			Name string `json:"name"`
		}
		inputJSON := block.JSON.Input.Raw()
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			output = fmt.Sprintf("Parse error: %v", err)
			isError = true
		} else {
			fmt.Printf("[remove_http_source] name=%s\n", input.Name)
			result, err := a.removeSource(input.Name)
			if err != nil {
				output = fmt.Sprintf("Failed to remove source: %v", err)
				isError = true
			} else {
				output = result
			}
		}

	case "list_http_sources":
		sources := a.fs.Sources()
		if len(sources) == 0 {
			output = "No HTTP sources currently configured."
		} else {
			var lines []string
			lines = append(lines, fmt.Sprintf("%d HTTP source(s) configured:", len(sources)))
			for name, url := range sources {
				lines = append(lines, fmt.Sprintf("  - %s: %s", name, url))
			}
			output = strings.Join(lines, "\n")
		}

	default:
		output = fmt.Sprintf("Unknown tool: %s", block.Name)
		isError = true
	}

	return anthropic.NewToolResultBlock(block.ID, output, isError)
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// runDemo runs a non-interactive demo showing dynamic mount capabilities
func runDemo(ctx context.Context, agent *DynamicAgent) {
	defer agent.fs.Stop()

	fmt.Println("=== Dynamic Mount Agent Demo (non-interactive mode) ===")
	fmt.Println()

	// Demo 1: List initial sources (should be empty)
	fmt.Println("--- Demo 1: Initial state (no sources) ---")
	agent.listSources()
	fmt.Println()

	// Demo 2: Let AI add an RSS feed
	fmt.Println("--- Demo 2: Ask AI to add an RSS feed ---")
	agent.chatWithLLM(ctx, "Add the GitHub commits feed for jackfish212/httpfs repository as 'httpfs-commits'")
	fmt.Println()

	// Wait for fetch
	fmt.Println("Waiting for feed to be fetched...")
	time.Sleep(3 * time.Second)
	fmt.Println()

	// Demo 3: List sources again
	fmt.Println("--- Demo 3: Sources after adding ---")
	agent.listSources()
	fmt.Println()

	// Demo 4: Explore via shell
	fmt.Println("--- Demo 4: Explore the feed via shell ---")
	result := agent.shell.Execute(ctx, "ls /http")
	fmt.Printf("ls /http:\n%s", result.Output)

	result = agent.shell.Execute(ctx, "ls /http/httpfs-commits")
	fmt.Printf("ls /http/httpfs-commits:\n%s", result.Output)
	fmt.Println()

	// Demo 5: Ask AI to summarize
	fmt.Println("--- Demo 5: Ask AI about the feed content ---")
	agent.chatWithLLM(ctx, "What are the latest commits in the httpfs-commits feed? Give me a brief summary.")
	fmt.Println()

	// Demo 6: Add another source
	fmt.Println("--- Demo 6: Add another source ---")
	agent.chatWithLLM(ctx, "Also add the Hacker News frontpage RSS feed as 'hacker-news'")
	fmt.Println()

	time.Sleep(2 * time.Second)

	// Demo 7: Final state
	fmt.Println("--- Demo 7: Final sources ---")
	agent.listSources()
	fmt.Println()

	fmt.Println("=== Demo complete ===")
	fmt.Println("Run with -i flag for interactive mode to add your own sources!")
}
