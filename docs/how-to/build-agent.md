# Build an Agent with Shell Routing

This guide shows how to build an AI agent that integrates GRASP with an LLM backend. The agent routes commands starting with `!` directly to the shell, and handles other input through the LLM.

## Architecture Overview

```
User Input
    │
    ├── "!xxx" → Shell.Execute("xxx") → Output
    │
    └── "other" → LLM → Tool Calls → Shell/Files → Response
```

## Basic Agent Setup

### Step 1: Create VirtualOS and Shell

```go
package main

import (
    "context"
    "fmt"
    "os"
    "strings"

    "github.com/jackfish212/grasp"
    "github.com/jackfish212/grasp/builtins"
    "github.com/jackfish212/grasp/mounts"
)

func main() {
    // Create and configure VirtualOS
    v := grasp.New()
    rootFS, err := grasp.Configure(v)
    if err != nil {
        panic(err)
    }
    builtins.RegisterBuiltinsOnFS(v, rootFS)

    // Mount data sources
    v.Mount("/data", mounts.NewLocalFS(".", grasp.PermRW))
    v.Mount("/memory", mounts.NewMemFS(grasp.PermRW))

    // Create shell
    sh := v.Shell("agent")
    ctx := context.Background()

    // Run agent loop
    runAgentLoop(ctx, sh)
}
```

### Step 2: Route Commands

```go
func runAgentLoop(ctx context.Context, sh *grasp.Shell) {
    reader := bufio.NewReader(os.Stdin)

    for {
        fmt.Print("> ")
        input, _ := reader.ReadString('\n')
        input = strings.TrimSpace(input)

        if input == "" {
            continue
        }
        if input == "exit" || input == "quit" {
            break
        }

        // Route: !xxx → shell, otherwise → LLM
        if strings.HasPrefix(input, "!") {
            cmd := strings.TrimPrefix(input, "!")
            result := sh.Execute(ctx, cmd)
            fmt.Println(result.Output)
        } else {
            response := callLLM(ctx, sh, input)
            fmt.Println(response)
        }
    }
}
```

### Step 3: Define Shell Tool for LLM

```go
func callLLM(ctx context.Context, sh *grasp.Shell, userMessage string) string {
    // Define the shell tool
    shellTool := anthropic.ToolParam{
        Name:        anthropic.F("shell"),
        Description: anthropic.F("Execute a shell command in the virtual filesystem"),
        InputSchema: anthropic.F(map[string]any{
            "type": "object",
            "properties": map[string]any{
                "command": map[string]any{
                    "type":        "string",
                    "description": "Shell command to execute",
                },
            },
            "required": []string{"command"},
        }),
    }

    // Create message with tool
    message, err := client.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
        MaxTokens: anthropic.F(int64(4096)),
        Tools:     anthropic.F([]anthropic.ToolParam{shellTool}),
        Messages: anthropic.F([]anthropic.MessageParam{
            anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
        }),
    })

    // Handle tool calls
    for _, block := range message.Content {
        if block.Type == anthropic.ContentBlockTypeToolUse {
            result := handleToolCall(ctx, sh, block)
            // ... return result to LLM and continue
        }
    }
}

func handleToolCall(ctx context.Context, sh *grasp.Shell, block anthropic.ContentBlock) string {
    var input struct {
        Command string `json:"command"`
    }
    json.Unmarshal(block.Input, &input)

    result := sh.Execute(ctx, input.Command)
    return result.Output
}
```

## Complete Example with Anthropic SDK

```go
package main

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "strings"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/jackfish212/grasp"
    "github.com/jackfish212/grasp/builtins"
    "github.com/jackfish212/grasp/mounts"
)

type Agent struct {
    client *anthropic.Client
    shell  *grasp.Shell
}

func main() {
    // Setup VirtualOS
    v := grasp.New()
    rootFS, _ := grasp.Configure(v)
    builtins.RegisterBuiltinsOnFS(v, rootFS)

    // Mount providers
    v.Mount("/data", mounts.NewLocalFS(".", grasp.PermRW))
    v.Mount("/memory", mounts.NewMemFS(grasp.PermRW))

    // Optional: Mount GitHub
    if token := os.Getenv("GITHUB_TOKEN"); token != "" {
        v.Mount("/github", mounts.NewGitHubFS(mounts.WithGitHubToken(token)))
    }

    // Create agent
    agent := &Agent{
        client: anthropic.NewClient(),
        shell:  v.Shell("agent"),
    }

    agent.Run(context.Background())
}

func (a *Agent) Run(ctx context.Context) {
    reader := bufio.NewReader(os.Stdin)
    var messages []anthropic.MessageParam

    fmt.Println("GRASP Agent - Type !<cmd> for shell, otherwise chat with AI")
    fmt.Println()

    for {
        fmt.Print("> ")
        input, _ := reader.ReadString('\n')
        input = strings.TrimSpace(input)

        if input == "" {
            continue
        }
        if input == "exit" || input == "quit" {
            break
        }

        // Direct shell routing
        if strings.HasPrefix(input, "!") {
            cmd := strings.TrimPrefix(input, "!")
            result := a.shell.Execute(ctx, cmd)
            if result.Code != 0 {
                fmt.Printf("Error (exit %d): %s", result.Code, result.Output)
            } else {
                fmt.Println(result.Output)
            }
            continue
        }

        // LLM routing
        messages = append(messages, anthropic.NewUserMessage(
            anthropic.NewTextBlock(input),
        ))

        response := a.callLLM(ctx, messages)
        fmt.Println(response)

        messages = append(messages, anthropic.NewAssistantMessage(
            anthropic.NewTextBlock(response),
        ))
    }
}

func (a *Agent) callLLM(ctx context.Context, messages []anthropic.MessageParam) string {
    toolDefinitions := []anthropic.ToolParam{
        {
            Name:        anthropic.F("shell"),
            Description: anthropic.F("Execute a shell command. Supports pipes, redirects, and composition."),
            InputSchema: anthropic.F(map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "command": map[string]any{
                        "type":        "string",
                        "description": "Shell command to execute",
                    },
                },
                "required": []string{"command"},
            }),
        },
        {
            Name:        anthropic.F("read"),
            Description: anthropic.F("Read a file from the virtual filesystem"),
            InputSchema: anthropic.F(map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "path": map[string]any{
                        "type":        "string",
                        "description": "File path to read",
                    },
                },
                "required": []string{"path"},
            }),
        },
    }

    for {
        message, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
            Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
            MaxTokens: anthropic.F(int64(4096)),
            Tools:     anthropic.F(toolDefinitions),
            Messages:  anthropic.F(messages),
            System: anthropic.F([]anthropic.TextBlockParam{
                {
                    Text: anthropic.F(`You are an AI agent with access to a virtual filesystem.

Available mount points:
- /data - Current working directory
- /memory - In-memory scratch space
- /github - GitHub API (if configured)

Use shell commands to explore and manipulate files. Examples:
- ls /data - list files
- cat /data/readme.md - read a file
- grep pattern /data/*.go - search for patterns
- search "query" --scope /data - full-text search`),
                },
            }),
        })

        if err != nil {
            return fmt.Sprintf("Error: %v", err)
        }

        // Check for tool calls
        var toolResults []anthropic.ToolResultBlockParam
        var hasToolCalls bool

        for _, block := range message.Content {
            if block.Type == anthropic.ContentBlockTypeToolUse {
                hasToolCalls = true
                result := a.executeTool(ctx, block)
                toolResults = append(toolResults, result)
            }
        }

        if !hasToolCalls {
            // No tool calls - return text response
            var textParts []string
            for _, block := range message.Content {
                if block.Type == anthropic.ContentBlockTypeText {
                    textParts = append(textParts, block.Text)
                }
            }
            return strings.Join(textParts, "\n")
        }

        // Add assistant message and tool results, then continue
        messages = append(messages, message.ToMessage())
        messages = append(messages, anthropic.NewUserMessage(toolResults...))
    }
}

func (a *Agent) executeTool(ctx context.Context, block anthropic.ContentBlock) anthropic.ToolResultBlockParam {
    var output string
    isError := false

    switch block.Name {
    case "shell":
        var input struct {
            Command string `json:"command"`
        }
        if err := json.Unmarshal(block.Input, &input); err != nil {
            output = fmt.Sprintf("Parse error: %v", err)
            isError = true
        } else {
            result := a.shell.Execute(ctx, input.Command)
            output = result.Output
            if result.Code != 0 {
                isError = true
            }
        }

    case "read":
        var input struct {
            Path string `json:"path"`
        }
        if err := json.Unmarshal(block.Input, &input); err != nil {
            output = fmt.Sprintf("Parse error: %v", err)
            isError = true
        } else {
            file, err := a.shell.VOS().Open(ctx, input.Path)
            if err != nil {
                output = fmt.Sprintf("Read error: %v", err)
                isError = true
            } else {
                defer file.Close()
                data, _ := io.ReadAll(file)
                output = string(data)
            }
        }

    default:
        output = fmt.Sprintf("Unknown tool: %s", block.Name)
        isError = true
    }

    return anthropic.NewToolResultBlock(block.ID, output, isError)
}
```

## Mounting All Provider Types

For a comprehensive setup with all available providers:

```go
func setupAllProviders(v *grasp.VirtualOS) {
    // Local filesystem
    v.Mount("/data", mounts.NewLocalFS(".", grasp.PermRW))

    // In-memory workspace
    memFS := mounts.NewMemFS(grasp.PermRW)
    v.Mount("/memory", memFS)

    // GitHub API
    if token := os.Getenv("GITHUB_TOKEN"); token != "" {
        v.Mount("/github", mounts.NewGitHubFS(
            mounts.WithGitHubToken(token),
        ))
    }

    // HTTP endpoints (requires separate package: github.com/jackfish212/httpfs)
    // httpFS := httpfs.NewHTTPFS()
    // httpFS.Add("news", "https://feeds.example.com/rss", &httpfs.RSSParser{})
    // v.Mount("/http", httpFS)

    // MCP servers
    if mcpCmd := os.Getenv("MCP_FILESYSTEM_CMD"); mcpCmd != "" {
        client := mounts.NewStdioMCPClient(mcpCmd)
        mounts.MountMCP(v, "/mcp/fs", client)
    }

    // HTTP MCP server
    if mcpURL := os.Getenv("MCP_HTTP_URL"); mcpURL != "" {
        client := mounts.NewHttpMCPClient(mcpURL,
            mounts.WithBearerToken(os.Getenv("MCP_HTTP_TOKEN")))
        mounts.MountMCP(v, "/mcp/remote", client)
    }
}
```

## System Prompt Template

A good system prompt helps the agent understand its environment:

```
You are an AI agent with shell access to a virtual filesystem.

## Available Mount Points

- /data - Project files (read/write)
- /memory - Temporary workspace (read/write)
- /persist - Persistent storage (survives restarts)
- /github - GitHub API (read-only)
- /http - HTTP endpoints as files (read-only)
- /mcp - MCP tools and resources

## Shell Commands

You have access to standard Unix-like commands:
- Navigation: ls, cd, pwd, find
- File operations: cat, read, write, mkdir, rm, mv
- Search: grep, search
- System: mount, which, uname, stat

Use pipes and composition: cat /data/log.md | grep error | head -5

## Best Practices

1. Explore before modifying: ls, cat, stat
2. Use /memory for temporary outputs
3. Use /persist for data that must survive
4. Chain commands with pipes for efficiency
```

## Tips

1. **Tool result limits**: LLMs have token limits. Truncate large outputs:
   ```go
   if len(output) > 10000 {
       output = output[:10000] + "\n... (truncated)"
   }
   ```

2. **Error handling**: Return errors as tool results so the LLM can retry:
   ```go
   if result.Code != 0 {
       isError = true
   }
   ```

3. **Context management**: Keep message history reasonable:
   ```go
   if len(messages) > 20 {
       messages = messages[2:] // Drop old messages
   }
   ```

4. **Timeout handling**: Use context with timeout for long operations:
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   result := sh.Execute(ctx, command)
   ```
