// Example: Using Shellfish with Mem0 API (HTTP direct)
//
// This example demonstrates how to use Mem0's REST API directly
// for persistent memory capabilities. No local Python required.
//
// Prerequisites:
//   - Get Mem0 API key from: https://app.mem0.ai
//   - Copy .env.example to .env and fill in credentials
//
// Run:
//
//	go run ./examples/mem0-mcp
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/builtins"
	"github.com/jackfish212/shellfish/mounts"
	"github.com/joho/godotenv"
)

// Mem0HTTPClient implements mounts.MCPClient using Mem0 REST API
type Mem0HTTPClient struct {
	apiKey string
	userID string
	client *http.Client
}

func NewMem0HTTPClient(apiKey, userID string) *Mem0HTTPClient {
	return &Mem0HTTPClient{
		apiKey: apiKey,
		userID: userID,
		client: &http.Client{},
	}
}

// Implement MCPClient interface with static tool definitions

func (c *Mem0HTTPClient) ListTools(ctx context.Context) ([]mounts.MCPTool, error) {
	return []mounts.MCPTool{
		{
			Name:        "add_memory",
			Description: "Store a new memory",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"text": map[string]any{
						"type":        "string",
						"description": "The memory content to store",
					},
				},
				"required": []string{"text"},
			},
		},
		{
			Name:        "search_memories",
			Description: "Search memories semantically",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "get_memories",
			Description: "List all memories",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "delete_memory",
			Description: "Delete a memory by ID",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"memory_id": map[string]any{
						"type":        "string",
						"description": "Memory ID to delete",
					},
				},
				"required": []string{"memory_id"},
			},
		},
	}, nil
}

func (c *Mem0HTTPClient) CallTool(ctx context.Context, name string, args map[string]any) (*mounts.MCPToolResult, error) {
	var result string
	var err error

	switch name {
	case "add_memory":
		result, err = c.addMemory(args["text"].(string))
	case "search_memories":
		result, err = c.searchMemories(args["query"].(string))
	case "get_memories":
		result, err = c.getMemories()
	case "delete_memory":
		result, err = c.deleteMemory(args["memory_id"].(string))
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}

	if err != nil {
		return &mounts.MCPToolResult{
			Content: []mounts.MCPContent{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
			IsError: true,
		}, nil
	}

	return &mounts.MCPToolResult{
		Content: []mounts.MCPContent{{Type: "text", Text: result}},
	}, nil
}

func (c *Mem0HTTPClient) addMemory(text string) (string, error) {
	payload := map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": text},
		},
		"user_id": c.userID,
	}

	body, err := c.doRequest("POST", "/v1/memories/", payload)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Mem0HTTPClient) searchMemories(query string) (string, error) {
	payload := map[string]any{
		"query": query,
		"filters": map[string]any{
			"user_id": c.userID,
		},
	}

	body, err := c.doRequest("POST", "/v2/memories/search/", payload)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Mem0HTTPClient) getMemories() (string, error) {
	body, err := c.doRequest("GET", "/v1/memories/?user_id="+c.userID, nil)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Mem0HTTPClient) deleteMemory(memoryID string) (string, error) {
	_, err := c.doRequest("DELETE", "/v1/memories/"+memoryID+"/?user_id="+c.userID, nil)
	if err != nil {
		return "", err
	}
	return `{"status": "deleted"}`, nil
}

func (c *Mem0HTTPClient) doRequest(method, path string, body any) ([]byte, error) {
	var req *http.Request
	var err error

	url := "https://api.mem0.ai" + path

	if body != nil {
		jsonBody, _ := json.Marshal(body)
		req, err = http.NewRequest(method, url, strings.NewReader(string(jsonBody)))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Token "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return json.MarshalIndent(json.RawMessage(result), "", "  ")
}

// Stub implementations for unused MCPClient methods
func (c *Mem0HTTPClient) ListResources(ctx context.Context) ([]mounts.MCPResource, error) {
	return nil, nil
}
func (c *Mem0HTTPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	return "", nil
}
func (c *Mem0HTTPClient) ListPrompts(ctx context.Context) ([]mounts.MCPPrompt, error) {
	return nil, nil
}
func (c *Mem0HTTPClient) GetPrompt(ctx context.Context, name string, args map[string]any) (string, error) {
	return "", nil
}

func main() {
	interactive := flag.Bool("i", false, "Run in interactive mode")
	flag.Parse()

	// Load .env file
	envPath := filepath.Join(".", ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
		log.Println("Using environment variables instead.")
	}

	ctx := context.Background()

	// Initialize Shellfish VirtualOS
	v := shellfish.New()
	rootFS, err := shellfish.Configure(v)
	if err != nil {
		log.Fatalf("Configure VOS: %v", err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// Create Mem0 HTTP client (no Python required!)
	apiKey := os.Getenv("MEM0_API_KEY")
	if apiKey == "" {
		log.Fatal("MEM0_API_KEY environment variable is required")
	}
	userID := os.Getenv("MEM0_USER_ID")
	if userID == "" {
		userID = "shellfish-user"
	}

	mem0Client := NewMem0HTTPClient(apiKey, userID)
	fmt.Println("Connected to Mem0 API (direct HTTP)")

	// Mount mem0 tools into VOS
	if err := mounts.MountMCP(v, "/mem0", mem0Client); err != nil {
		log.Fatalf("Mount mem0: %v", err)
	}

	// List available tools
	tools, err := mem0Client.ListTools(ctx)
	if err != nil {
		log.Printf("Warning: Could not list tools: %v", err)
	} else {
		fmt.Println("\nAvailable Mem0 tools:")
		for _, t := range tools {
			fmt.Printf("  - %s: %s\n", t.Name, truncate(t.Description, 60))
		}
	}

	// Initialize Anthropic client
	client := newAnthropicClient()

	// Define shell tool (for VOS access) and memory tools
	shellTool := anthropic.ToolParam{
		Name:        "shell",
		Description: anthropic.String("Execute a shell command in the virtual filesystem."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute",
				},
			},
			Required: []string{"command"},
		},
	}

	// Define memory tool that wraps mem0-mcp calls
	memoryTool := anthropic.ToolParam{
		Name:        "memory",
		Description: anthropic.String("Store and retrieve memories using Mem0. Use this to remember user preferences, past conversations, and important information."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
			Properties: map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"add", "search", "list", "delete"},
					"description": "The memory action to perform",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "For 'add': the content to remember. For 'search': the search query.",
				},
				"memory_id": map[string]interface{}{
					"type":        "string",
					"description": "For 'delete': the memory ID to delete",
				},
			},
			Required: []string{"action"},
		},
	}

	fmt.Println("\n========================================")
	fmt.Println("  Shellfish + Mem0 MCP Integration")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("The AI agent can:")
	fmt.Println("  - Store memories about conversations and preferences")
	fmt.Println("  - Search and retrieve past memories")
	fmt.Println("  - Execute shell commands in the virtual filesystem")
	fmt.Println()

	if *interactive {
		runInteractiveMode(ctx, v, mem0Client, client, shellTool, memoryTool)
	} else {
		runDemoMode(ctx, v, mem0Client, client, shellTool, memoryTool)
	}
}

func runInteractiveMode(ctx context.Context, v *shellfish.VirtualOS, mem0Client mounts.MCPClient, client anthropic.Client, shellTool, memoryTool anthropic.ToolParam) {
	fmt.Println("Interactive Mode")
	fmt.Println("=============== ")
	fmt.Println("Type your message and press Enter to chat.")
	fmt.Println("The agent has access to persistent memory via Mem0.")
	fmt.Println("Press Ctrl+C or type 'exit' to quit.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	messages := []anthropic.MessageParam{}

	systemPrompt := `You are an AI assistant with two key capabilities:

1. MEMORY: You have persistent memory via Mem0. Use the 'memory' tool to:
   - Remember important information about the user (preferences, facts)
   - Store insights from conversations
   - Retrieve relevant memories when needed
   - Always check memory at the start to recall past context

2. SHELL: You can execute commands in a virtual filesystem.

Memory actions:
- add: Store new information (--content "what to remember")
- search: Find relevant memories (--content "search query")
- list: Show all memories
- delete: Remove a memory (--memory-id "id")

Be proactive about storing and retrieving memories to provide personalized assistance.`

	for {
		fmt.Print("You: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
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
		messages = processWithAgent(ctx, v, mem0Client, client, shellTool, memoryTool, messages, systemPrompt)
	}
}

func runDemoMode(ctx context.Context, v *shellfish.VirtualOS, mem0Client mounts.MCPClient, client anthropic.Client, shellTool, memoryTool anthropic.ToolParam) {
	fmt.Println("Demo Mode")
	fmt.Println("=========")
	fmt.Println()

	// First, let's demonstrate using mem0 tools directly via shell
	fmt.Println("1. Testing direct mem0 access via shell:")
	sh := v.Shell("demo")

	fmt.Println("\n   Adding a memory...")
	result := sh.Execute(ctx, "/mem0/tools/add-memory --text \"User prefers dark mode in all applications\"")
	fmt.Printf("   Result: %s\n", truncate(result.Output, 200))

	fmt.Println("\n   Adding another memory...")
	result = sh.Execute(ctx, "/mem0/tools/add-memory --text \"User's favorite programming language is Go\"")
	fmt.Printf("   Result: %s\n", truncate(result.Output, 200))

	fmt.Println("\n   Searching memories for 'programming'...")
	result = sh.Execute(ctx, "/mem0/tools/search-memories --query \"programming\"")
	fmt.Printf("   Result: %s\n", truncate(result.Output, 500))

	fmt.Println("\n2. Now demonstrating AI agent with memory:")
	fmt.Println("   Task: Remember user preferences and use them in responses")
	fmt.Println()

	task := `Please perform the following tasks:

1. First, search your memory for any existing information about this user
2. If you find preferences, acknowledge them
3. Store a new memory: the user is working on a project called "Shellfish"
4. Search your memory again to confirm the new memory was stored
5. Summarize all the memories you found about this user

Use the memory tool for all memory operations.`

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(task)),
	}

	systemPrompt := `You are an AI assistant with persistent memory capabilities via Mem0.

Use the 'memory' tool to:
- Remember information (--action add --content "what to remember")
- Search memories (--action search --content "query")
- List all memories (--action list)
- Delete memories (--action delete --memory-id "id")

Always check memory first to provide personalized responses based on past interactions.`

	processWithAgent(ctx, v, mem0Client, client, shellTool, memoryTool, messages, systemPrompt)

	fmt.Println("\n[Demo Complete]")
}

func processWithAgent(ctx context.Context, v *shellfish.VirtualOS, mem0Client mounts.MCPClient, client anthropic.Client, shellTool, memoryTool anthropic.ToolParam, messages []anthropic.MessageParam, systemPrompt string) []anthropic.MessageParam {
	sh := v.Shell("agent")
	model := getModel()

	for {
		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: 4096,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  messages,
			Tools:     []anthropic.ToolUnionParam{{OfTool: &shellTool}, {OfTool: &memoryTool}},
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
				var result string

				switch b.Name {
				case "shell":
					var input struct {
						Command string `json:"command"`
					}
					if err := json.Unmarshal([]byte(b.JSON.Input.Raw()), &input); err != nil {
						result = fmt.Sprintf("Error parsing input: %v", err)
					} else {
						fmt.Printf("\n[shell] %s\n", input.Command)
						execResult := sh.Execute(ctx, input.Command)
						result = execResult.Output
						if execResult.Code != 0 {
							result += fmt.Sprintf("\n[exit code: %d]", execResult.Code)
						}
						fmt.Printf("[result] %s\n", truncate(result, 300))
					}

				case "memory":
					var input struct {
						Action    string `json:"action"`
						Content   string `json:"content"`
						MemoryID  string `json:"memory_id"`
					}
					if err := json.Unmarshal([]byte(b.JSON.Input.Raw()), &input); err != nil {
						result = fmt.Sprintf("Error parsing input: %v", err)
					} else {
						fmt.Printf("\n[memory] %s", input.Action)
						result = executeMemoryAction(ctx, mem0Client, input.Action, input.Content, input.MemoryID)
						fmt.Printf("\n[result] %s\n", truncate(result, 300))
					}
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, result, false))
			}
		}

		if !hasToolUse {
			break
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	return messages
}

func executeMemoryAction(ctx context.Context, client mounts.MCPClient, action, content, memoryID string) string {
	switch action {
	case "add":
		if content == "" {
			return "Error: content is required for add action"
		}
		result, err := client.CallTool(ctx, "add_memory", map[string]any{"text": content})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return formatToolResult(result)

	case "search":
		if content == "" {
			return "Error: content (query) is required for search action"
		}
		result, err := client.CallTool(ctx, "search_memories", map[string]any{"query": content})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return formatToolResult(result)

	case "list":
		result, err := client.CallTool(ctx, "get_memories", map[string]any{})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return formatToolResult(result)

	case "delete":
		if memoryID == "" {
			return "Error: memory_id is required for delete action"
		}
		result, err := client.CallTool(ctx, "delete_memory", map[string]any{"memory_id": memoryID})
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return formatToolResult(result)

	default:
		return fmt.Sprintf("Unknown action: %s", action)
	}
}

func formatToolResult(result *mounts.MCPToolResult) string {
	var texts []string
	for _, c := range result.Content {
		if c.Text != "" {
			texts = append(texts, c.Text)
		}
	}
	output := strings.Join(texts, "\n")
	if result.IsError {
		output = "[Error] " + output
	}
	return output
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

func getModel() anthropic.Model {
	if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
		return anthropic.Model(m)
	}
	return anthropic.ModelClaudeSonnet4_5_20250929
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "... (truncated)"
}
