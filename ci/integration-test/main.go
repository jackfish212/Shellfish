// Integration Test: Self-Validation Agent with GitHub MCP Issue Creation
//
// This test mounts various filesystem providers and has an AI agent
// validate the current system. Issues are reported via GitHub MCP tools directly.
//
// Run:
//
//	cd ci/integration-test
//	go run .
//
// Environment variables:
//
//	GITHUB_TOKEN       - GitHub token with repo scope (required)
//	GITHUB_MCP_URL     - GitHub MCP server URL (optional)
//	ANTHROPIC_AUTH_TOKEN - Anthropic API token (required)
//	ANTHROPIC_BASE_URL - Anthropic API base URL (optional)
//	GITHUB_REPOSITORY  - Repository to create issues in (e.g., owner/repo)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mounts"
)

const maxIterations = 60

func main() {
	ctx := context.Background()

	// Initialize grasp VirtualOS
	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		log.Fatalf("Configure VOS: %v", err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// 1. Mount Memory FS
	memFS := mounts.NewMemFS(grasp.PermRW)
	if err := v.Mount("/memfs", memFS); err != nil {
		log.Fatalf("Mount /memfs: %v", err)
	}
	setupTestFiles(v)

	// 2. Mount GitHub MCP
	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	mcpURL := os.Getenv("GITHUB_MCP_URL")
	if mcpURL == "" {
		mcpURL = "https://api.githubcopilot.com/mcp/"
	}

	mcpClient := mounts.NewHttpMCPClient(mcpURL, mounts.WithBearerToken(githubToken))

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

	// List available GitHub tools
	tools, err := mcpClient.ListTools(ctx)
	if err != nil {
		log.Printf("Warning: Could not list tools: %v", err)
	} else {
		fmt.Printf("\nGitHub MCP tools available: %d\n", len(tools))
		// Find issue-related tools
		for _, t := range tools {
			if strings.Contains(strings.ToLower(t.Name), "issue") {
				fmt.Printf("  - %s: %s\n", t.Name, truncate(t.Description, 60))
			}
		}
	}

	// 3. Mount workspace for output
	workFS := mounts.NewMemFS(grasp.PermRW)
	if err := v.Mount("/workspace", workFS); err != nil {
		log.Fatalf("Mount /workspace: %v", err)
	}

	// Initialize Anthropic client
	client := newAnthropicClient()
	shellTool := shellToolDef()

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  grasp Integration Test Agent")
	fmt.Println("========================================")
	fmt.Println()

	// Get repository info
	repo := os.Getenv("GITHUB_REPOSITORY")
	var owner, repoName string
	if repo != "" {
		parts := strings.Split(repo, "/")
		if len(parts) == 2 {
			owner, repoName = parts[0], parts[1]
		}
	}

	// Run the validation task
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(validationTask(owner, repoName))),
	}

	systemPrompt := buildSystemPrompt(owner, repoName)
	runValidationLoop(ctx, v, client, shellTool, messages, systemPrompt)

	fmt.Println()
	fmt.Println("Validation complete.")

	// Output results summary
	sh := v.Shell("viewer")
	result := sh.Execute(ctx, "cat /workspace/report.md")
	if result.Output != "" {
		fmt.Println("\n=== Validation Report ===")
		fmt.Println(result.Output)
	}
}

func setupTestFiles(v *grasp.VirtualOS) {
	ctx := context.Background()

	// Create test files for validation
	v.Write(ctx, "/memfs/test.txt", strings.NewReader("Hello, World!\n"))
	v.Write(ctx, "/memfs/config.json", strings.NewReader(`{"name": "grasp", "version": "1.0.0"}`))
	v.Write(ctx, "/memfs/README.md", strings.NewReader("# Test Directory\n\nThis is a test directory for validation.\n"))
}

func validationTask(owner, repo string) string {
	var issueCreation string
	if owner != "" && repo != "" {
		issueCreation = fmt.Sprintf(`
### 5. Create GitHub Issues for Problems Found

If you find any CRITICAL or HIGH severity issues, create GitHub issues directly using the MCP tools.

Example command to create an issue:
`+"```"+`
/github/tools/create-issue --owner %s --repo %s --title "Issue Title" --body "Issue description..." --labels "bug,automated-test"
`+"```"+`

Or use the appropriate tool name (check with: ls /github/tools/ | grep issue).

IMPORTANT: Actually execute the create-issue command for each critical/high issue found.
`, owner, repo)
	} else {
		issueCreation = `
### 5. Issue Reporting (No GitHub Repository Configured)

Since GITHUB_REPOSITORY is not set, list the issues at the end of your report instead of creating GitHub issues.`
	}

	return fmt.Sprintf(`You are a validation agent. Your task is to thoroughly test the grasp virtual filesystem and report any issues.

## Filesystems Available

1. **Memory FS** at /memfs/ - A test filesystem with sample files
2. **GitHub MCP** at /github/tools/ - GitHub API tools via MCP
3. **Workspace** at /workspace/ - Write your report here

## Validation Tasks

### 1. Basic File Operations (Memory FS)
- List files in /memfs/
- Read /memfs/test.txt and verify content
- Read /memfs/config.json and verify JSON is valid
- Create a new file /memfs/output.txt with test content
- Test append operation to /memfs/output.txt
- Use grep to search for patterns
- Use wc to count lines/words

### 2. Shell Features
- Test pipes: cat /memfs/test.txt | wc -l
- Test redirections: echo "test" > /memfs/new.txt
- Test cd and pwd commands
- Test find command

### 3. GitHub MCP Connection
- List available tools with: ls /github/tools/
- Find issue-related tools: ls /github/tools/ | grep -i issue
- Report the number of available tools
- Try to search repositories or get repo info (if permissions allow)

### 4. Error Handling
- Try to read a non-existent file and verify error handling
- Try invalid commands and verify errors are caught
%s
## Output Requirements

After validation, create a report at /workspace/report.md with:
1. Summary of tests performed
2. List of PASSED tests
3. List of FAILED tests (if any)
4. List of issues found (if any)

For each issue, include:
- **ISSUE**: [brief title]
- **SEVERITY**: [critical/high/medium/low]
- **DETAILS**: [detailed explanation]

Complete all validation tasks before writing the report. If you find critical or high severity issues and have GitHub access, create issues immediately using the /github/tools/ commands.`, issueCreation)
}

func buildSystemPrompt(owner, repo string) string {
	prompt := `You are a validation agent testing the grasp virtual filesystem.

You have access to shell commands through the 'shell' tool. Use it to execute commands in the virtual filesystem.

Available commands include:
- ls, cat, read, write, stat, grep, find, head, tail, mkdir, rm, mv, echo, pwd, cd, wc
- Pipes (|) and redirects (>, >>) are supported
- /github/tools/ contains MCP tools for GitHub API

Be thorough and systematic. Document all test results. Report any unexpected behavior or errors as issues.

To use GitHub MCP tools:
1. First, list available tools: ls /github/tools/
2. Read tool help by reading the tool file: cat /github/tools/create-issue
3. Execute tools with flags: /github/tools/create-issue --owner X --repo Y --title "..." --body "..."

When creating GitHub issues:
- Use clear, descriptive titles
- Include reproduction steps in the body
- Add appropriate labels (bug, automated-test)
- Be professional and factual`

	if owner != "" && repo != "" {
		prompt += fmt.Sprintf(`

Target repository for issues: %s/%s`, owner, repo)
	}

	return prompt
}

func runValidationLoop(ctx context.Context, v *grasp.VirtualOS, client anthropic.Client, tool anthropic.ToolParam, messages []anthropic.MessageParam, systemPrompt string) {
	sh := v.Shell("agent")
	model := getModel()
	iterations := 0

	for iterations < maxIterations {
		iterations++

		resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     model,
			MaxTokens: 4096,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  messages,
			Tools:     []anthropic.ToolUnionParam{{OfTool: &tool}},
		})
		if err != nil {
			log.Printf("API error: %v\n", err)
			break
		}

		messages = append(messages, resp.ToParam())

		var toolResults []anthropic.ContentBlockParamUnion
		hasToolUse := false

		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				fmt.Print(".")

			case anthropic.ToolUseBlock:
				hasToolUse = true
				var input struct {
					Command string `json:"command"`
				}
				json.Unmarshal([]byte(b.JSON.Input.Raw()), &input)

				// Show GitHub MCP commands
				if strings.HasPrefix(input.Command, "/github/tools/") {
					fmt.Printf("\n[GitHub MCP] %s\n", input.Command)
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

				// Show GitHub MCP results
				if strings.HasPrefix(input.Command, "/github/tools/") && len(output) > 0 {
					fmt.Printf("[Result] %s\n", truncate(output, 200))
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, output, result.Code != 0))
			}
		}

		if !hasToolUse {
			break
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))

		// Small delay between iterations
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println()
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
