// Issue CodeGen - AI-powered code generator for GitHub Issues
//
// This tool reads a GitHub issue, uses Anthropic Claude to understand
// the problem and generate code changes, then prepares files for a PR.
//
// Usage:
//
//	issue-codegen -repo owner/repo -issue 123 -title "Issue title" -body "Issue body" -workdir /path/to/repo
//
// Environment variables:
//
//	ANTHROPIC_API_KEY - Anthropic API key (required)
//	ANTHROPIC_BASE_URL - Optional custom API base URL
//	ANTHROPIC_MODEL - Optional model override
//	GITHUB_TOKEN - GitHub token for MCP access (optional)
//	GITHUB_MCP_URL - Optional custom GitHub MCP server URL
package main

import (
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
)

func main() {
	repo := flag.String("repo", "", "GitHub repository (owner/repo)")
	issueNum := flag.Int("issue", 0, "Issue number")
	title := flag.String("title", "", "Issue title")
	body := flag.String("body", "", "Issue body")
	workdir := flag.String("workdir", ".", "Working directory (repository root)")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if *repo == "" || *issueNum == 0 || *title == "" {
		log.Fatal("Usage: issue-codegen -repo owner/repo -issue N -title \"Title\" [-body \"Body\"] [-workdir path]")
	}

	ctx := context.Background()

	// Initialize grasp VirtualOS
	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		log.Fatalf("Configure VOS: %v", err)
	}
	if err := builtins.RegisterBuiltinsOnFS(v, rootFS); err != nil {
		log.Fatalf("Register builtins: %v", err)
	}

	// Mount the local repository
	absWorkdir, err := filepath.Abs(*workdir)
	if err != nil {
		log.Fatalf("Get absolute path: %v", err)
	}
	localFS := mounts.NewLocalFS(absWorkdir, grasp.PermRW)
	if err := v.Mount("/repo", localFS); err != nil {
		log.Fatalf("Mount /repo: %v", err)
	}

	// Mount a memory filesystem for scratch space
	memFS := mounts.NewMemFS(grasp.PermRW)
	if err := v.Mount("/scratch", memFS); err != nil {
		log.Fatalf("Mount /scratch: %v", err)
	}

	// Try to connect to GitHub MCP Server if token is available
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken != "" {
		mcpURL := os.Getenv("GITHUB_MCP_URL")
		if mcpURL == "" {
			mcpURL = "https://api.githubcopilot.com/mcp/"
		}

		mcpClient := mounts.NewHttpMCPClient(mcpURL, mounts.WithBearerToken(githubToken))
		if _, err := mcpClient.Initialize(ctx); err == nil {
			log.Println("Connected to GitHub MCP Server")
			if err := mounts.MountMCP(v, "/github", mcpClient); err != nil {
				log.Printf("Warning: Could not mount GitHub MCP: %v", err)
			}
		} else {
			log.Printf("Note: GitHub MCP not available: %v", err)
		}
	}

	// Create Anthropic client
	client := newAnthropicClient()

	// Build system prompt
	systemPrompt := buildSystemPrompt(*repo, *issueNum)

	// Build user message with issue details
	issueContext := fmt.Sprintf(`## GitHub Issue #%d

**Repository:** %s
**Title:** %s

**Description:**
%s

---
Please analyze this issue and implement the necessary changes in the repository at /repo/.
`, *issueNum, *repo, *title, *body)

	// Create shell tool definition
	shellTool := anthropic.ToolParam{
		Name:        "shell",
		Description: anthropic.String("Execute a shell command in the virtual filesystem. The repository is mounted at /repo/. Available commands: ls, cat, grep, find, head, tail, mkdir, rm, mv, cp, echo, write, wc. Use pipes (|) and redirects (>, >>) to chain commands. For writing code files, prefer using 'cat > /repo/path/to/file << 'EOF' ... EOF' syntax."),
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

	// Start agent loop
	fmt.Println("========================================")
	fmt.Println("  grasp Issue CodeGen Agent")
	fmt.Println("========================================")
	fmt.Printf("Repository: %s\n", *repo)
	fmt.Printf("Issue #%d: %s\n\n", *issueNum, *title)

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(issueContext)),
	}

	_ = agentLoop(ctx, v, client, shellTool, messages, systemPrompt, *verbose)

	fmt.Println("\n[Agent finished]")

	// Show summary of changes
	sh := v.Shell("summary")
	result := sh.Execute(ctx, "cd /repo && git diff --stat")
	if result.Output != "" {
		fmt.Println("\n=== Changes Summary ===")
		fmt.Println(result.Output)
	}

	result = sh.Execute(ctx, "cd /repo && git status --short")
	if result.Output != "" {
		fmt.Println("\n=== Modified Files ===")
		fmt.Println(result.Output)
	}
}

func buildSystemPrompt(repo string, issueNum int) string {
	return fmt.Sprintf(`You are an AI software engineer tasked with implementing code changes to address a GitHub issue.

## Your Task
Analyze the issue and implement the necessary code changes in the repository.

## Environment
- Repository is mounted at /repo/
- You have full read/write access to the repository
- Use standard shell commands to explore and modify files
- GitHub MCP tools may be available at /github/tools/ for additional context

## Workflow
1. **Explore**: First, understand the codebase structure
   - ls -la /repo/ to see root files
   - find /repo -type f -name "*.go" to find Go files
   - cat /repo/path/to/file.go to read files

2. **Analyze**: Read relevant files to understand the context
   - Identify which files need to be modified
   - Understand the existing patterns and conventions

3. **Implement**: Make the necessary code changes
   - Use cat > /repo/path/to/file << 'EOF' ... EOF for creating/modifying files
   - Or use echo for small changes
   - Preserve existing code style

4. **Verify**: Check that your changes are correct
   - Read back modified files to confirm
   - Ensure no syntax errors

## File Writing
For creating or modifying files, use heredoc syntax:
cat > /repo/path/to/file.go << 'EOF'
package main

// your code here
EOF

## Guidelines
- Make minimal, focused changes that directly address the issue
- Follow existing code style and conventions in the repository
- Add comments only where necessary for clarity
- Do NOT break existing functionality
- Do NOT add unnecessary dependencies
- Test files should be added if the change is significant

## Commands Available
- ls, cat, grep, find, head, tail - for exploration
- cat > file << 'EOF' ... EOF - to create/overwrite files
- mkdir -p, rm, mv, cp - file operations

## Important
- All file paths should start with /repo/
- Work directly on the files in /repo/
- When done, report a summary of what changes you made

Repository: %s
Issue Number: #%d`, repo, issueNum)
}

func agentLoop(ctx context.Context, v *grasp.VirtualOS, client anthropic.Client, tool anthropic.ToolParam, messages []anthropic.MessageParam, systemPrompt string, verbose bool) []anthropic.MessageParam {
	sh := v.Shell("agent")
	model := getModel()
	maxIterations := 100

	for i := 0; i < maxIterations; i++ {
		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: 8192,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
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
				if verbose || len(b.Text) < 200 {
					fmt.Printf("\n%s\n", b.Text)
				} else {
					fmt.Printf("\n%s...\n", truncate(b.Text, 200))
				}
			case anthropic.ToolUseBlock:
				hasToolUse = true
				var input struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal([]byte(b.JSON.Input.Raw()), &input); err != nil {
					continue
				}

				fmt.Printf("  $ %s\n", input.Command)

				result := sh.Execute(ctx, input.Command)
				output := result.Output
				if result.Code != 0 {
					if output != "" && !strings.HasSuffix(output, "\n") {
						output += "\n"
					}
					output += fmt.Sprintf("[exit code: %d]", result.Code)
				}

				if verbose && output != "" {
					fmt.Printf("  %s\n", indentLines(output, "  "))
				} else if output != "" {
					fmt.Printf("  %s\n", truncate(output, 300))
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

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY environment variable is required")
	}

	return anthropic.NewClient(option.WithAPIKey(apiKey))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Try to truncate at a newline
	truncated := s[:max]
	if idx := strings.LastIndex(truncated, "\n"); idx > max/2 {
		return truncated[:idx] + "\n... (truncated)"
	}
	return truncated + "... (truncated)"
}

func indentLines(s string, indent string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}
