// Example: GitHub Provider - Mount GitHub API as a Filesystem
//
// This example demonstrates how to create a custom Provider that exposes
// GitHub's REST API as a virtual filesystem. It showcases Shellfish's
// "Mount Anything" philosophy.
//
// Run: go run ./examples/github-provider
//
// Configuration: Set GITHUB_TOKEN environment variable for higher rate limits
// and access to private repositories. Without a token, you're limited to
// 60 requests/hour.
//
// Filesystem layout:
//
//	/repos                          - list repositories
//	/repos/{owner}/{repo}           - repository info
//	/repos/{owner}/{repo}/contents  - repository files
//	/repos/{owner}/{repo}/issues    - list issues
//	/repos/{owner}/{repo}/issues/N  - read issue #N
//
// Example commands:
//
//	ls /repos                           # list user's repos
//	ls /repos/golang/go                 # explore go repo
//	cat /repos/golang/go/README.md      # read file
//	cat /repos/golang/go/issues/123     # read issue
//	search "bug" --scope /repos/owner/repo/issues
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/builtins"
	"github.com/jackfish212/shellfish/mounts"
	"github.com/joho/godotenv"
)

func main() {
	// Parse command line flags
	interactive := flag.Bool("i", false, "Run in interactive mode")
	repo := flag.String("repo", "", "Default repository to explore (format: owner/repo)")
	flag.Parse()

	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Note: No .env file found, using environment variables")
	}

	// Initialize Shellfish VirtualOS
	v := shellfish.New()
	rootFS, err := shellfish.Configure(v)
	if err != nil {
		panic(err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// Create and mount GitHub filesystem
	githubOpts := []mounts.GitHubFSOption{}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		githubOpts = append(githubOpts, mounts.WithGitHubToken(token))
	}
	if *repo != "" {
		parts := strings.SplitN(*repo, "/", 2)
		if len(parts) == 2 {
			githubOpts = append(githubOpts, mounts.WithGitHubUser(parts[0]))
		}
	}

	githubFS := mounts.NewGitHubFS(githubOpts...)
	if err := v.Mount("/github", githubFS); err != nil {
		panic(fmt.Errorf("failed to mount /github: %w", err))
	}

	// Initialize Anthropic client
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")

	var client anthropic.Client
	if baseURL != "" && authToken != "" {
		client = anthropic.NewClient(
			option.WithBaseURL(baseURL),
			option.WithAPIKey(authToken),
		)
	} else {
		client = anthropic.NewClient()
	}

	// Define the shell tool
	shellTool := anthropic.ToolParam{
		Name:        "shell",
		Description: anthropic.String("Execute a shell command to navigate and interact with the GitHub filesystem. Commands: ls, cat, stat, grep, find, search. Mount point: /github/repos/"),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "Shell command (e.g., 'ls /github/repos/golang/go', 'cat /github/repos/golang/go/README.md', 'search \"bug\" --scope /github/repos/owner/repo/issues')",
				},
			},
			Required: []string{"command"},
		},
	}

	ctx := context.Background()

	fmt.Println("GitHub Provider Example")
	fmt.Println("=======================")
	fmt.Println("Mount: /github -> GitHub API")
	fmt.Println()
	fmt.Println("Available paths:")
	fmt.Println("  /github/repos                    - list repositories")
	fmt.Println("  /github/repos/{owner}/{repo}     - repository info")
	fmt.Println("  /github/repos/{owner}/{repo}/contents/... - files")
	fmt.Println("  /github/repos/{owner}/{repo}/issues - issues")
	fmt.Println()

	if *interactive {
		runInteractiveMode(ctx, v, client, shellTool)
	} else {
		runDefaultTask(ctx, v, client, shellTool, *repo)
	}
}

func runInteractiveMode(ctx context.Context, v *shellfish.VirtualOS, client anthropic.Client, shellTool anthropic.ToolParam) {
	fmt.Println("Interactive Mode")
	fmt.Println("=============== ")
	fmt.Println("Ask questions about GitHub repositories.")
	fmt.Println("Type 'exit' to quit.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	messages := []anthropic.MessageParam{}

	systemPrompt := `You are an AI assistant with access to GitHub through a virtual filesystem mounted at /github. Use shell commands to:

1. Explore repositories: ls /github/repos/{owner}/{repo}
2. Read files: cat /github/repos/{owner}/{repo}/contents/path/to/file
3. View issues: cat /github/repos/{owner}/{repo}/issues/{number}
4. Search issues: search "query" --scope /github/repos/{owner}/{repo}/issues

Be helpful and answer questions about code, documentation, issues, and project structure.`

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
		messages = processAgentLoop(ctx, v, client, shellTool, messages, systemPrompt)
	}
}

func runDefaultTask(ctx context.Context, v *shellfish.VirtualOS, client anthropic.Client, shellTool anthropic.ToolParam, defaultRepo string) {
	// Use a popular open-source project as example
	exampleRepo := "golang/go"
	if defaultRepo != "" {
		exampleRepo = defaultRepo
	}

	task := fmt.Sprintf(`Explore the GitHub repository at /github/repos/%s and provide a summary:

1. List the repository contents to understand its structure
2. Read the README.md to understand what the project does
3. Look at recent issues to understand common problems or feature requests
4. Summarize the project in 2-3 sentences

Use shell commands to explore the filesystem mounted at /github.`, exampleRepo)

	fmt.Printf("Task: Explore %s\n\n", exampleRepo)
	fmt.Println("Starting agent...")

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(task)),
	}

	systemPrompt := "You are an AI assistant exploring GitHub repositories through a virtual filesystem. Use shell commands to navigate /github and answer questions about code, documentation, and issues."
	messages = processAgentLoop(ctx, v, client, shellTool, messages, systemPrompt)

	fmt.Println("\n[Done]")
}

func processAgentLoop(ctx context.Context, v *shellfish.VirtualOS, client anthropic.Client, shellTool anthropic.ToolParam, messages []anthropic.MessageParam, systemPrompt string) []anthropic.MessageParam {
	for {
		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_5_20250929,
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    []anthropic.ToolUnionParam{{OfTool: &shellTool}},
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
				if b.Name == "shell" {
					var input struct {
						Command string `json:"command"`
					}
					inputJSON := b.JSON.Input.Raw()
					if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
						toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, fmt.Sprintf("Error parsing input: %v", err), true))
						continue
					}

					fmt.Printf("\n[shell] %s\n", input.Command)

					result := executeShell(v, input.Command)
					fmt.Printf("[result]\n%s\n", truncate(result, 1500))

					toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, result, false))
				}
			}
		}

		if !hasToolUse {
			break
		}

		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	return messages
}

func executeShell(v *shellfish.VirtualOS, command string) string {
	sh := v.Shell("agent")
	result := sh.Execute(context.Background(), command)

	var output strings.Builder
	if result.Output != "" {
		output.WriteString(result.Output)
	}
	if result.Code != 0 {
		if output.Len() > 0 && !strings.HasSuffix(output.String(), "\n") {
			output.WriteString("\n")
		}
		output.WriteString(fmt.Sprintf("exit code: %d", result.Code))
	}

	return output.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... (truncated)"
}
