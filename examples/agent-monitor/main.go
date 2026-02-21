// Example: Agent Monitor — watching user shell activity with VFS hooks
//
// This example demonstrates grasp's two reactive primitives:
//
//   - VOS.Watch  — inotify-style file change notifications
//   - Shell.OnExec — post-execution callback hooks
//
// A human user types commands in an interactive shell. An AI agent runs
// in the background, observing through these hooks. When a command fails
// or a watched file changes (e.g. .bash_history), the agent automatically
// offers contextual help.
//
// Run: go run ./examples/agent-monitor
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mounts"
	"github.com/jackfish212/grasp/shell"
	"github.com/joho/godotenv"
)

func main() {
	envPath := filepath.Join(".", ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("Warning: could not load .env: %v", err)
	}

	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		log.Fatalf("configure: %v", err)
	}
	if err := builtins.RegisterBuiltinsOnFS(v, rootFS); err != nil {
		log.Fatalf("register builtins: %v", err)
	}

	workspace := mounts.NewMemFS(grasp.PermRW)
	if err := v.Mount("/workspace", workspace); err != nil {
		log.Fatalf("mount /workspace: %v", err)
	}
	seedWorkspace(v)

	client := newAnthropicClient()
	sh := v.Shell("user")

	monitor := newAgentMonitor(v, sh, client)
	monitor.start()
	defer monitor.stop()

	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║          grasp Agent Monitor Demo                   ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Println("║  You type shell commands; an AI agent watches silently. ║")
	fmt.Println("║  When you hit an error or edit watched files,           ║")
	fmt.Println("║  the agent will automatically offer help.               ║")
	fmt.Println("║                                                         ║")
	fmt.Println("║  Try:  cat /workspace/nonexistent.txt                   ║")
	fmt.Println("║        ls /workspace                                    ║")
	fmt.Println("║        echo hello > /workspace/notes.txt                ║")
	fmt.Println("║        badcommand                                       ║")
	fmt.Println("║                                                         ║")
	fmt.Println("║  Type 'exit' to quit.                                   ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()

	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("\033[1;32muser@grasp\033[0m:\033[1;34m%s\033[0m$ ", sh.Cwd())
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		result := sh.Execute(ctx, input)
		if result.Output != "" {
			fmt.Print(result.Output)
		}
	}
}

// ─── Agent Monitor ───

type agentMonitor struct {
	v        *grasp.VirtualOS
	sh       *grasp.Shell
	client   anthropic.Client
	messages []anthropic.MessageParam
	watcher  *grasp.Watcher
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
}

func newAgentMonitor(v *grasp.VirtualOS, sh *grasp.Shell, client anthropic.Client) *agentMonitor {
	return &agentMonitor{
		v:      v,
		sh:     sh,
		client: client,
	}
}

func (m *agentMonitor) start() {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// Hook 1: watch command execution results
	m.sh.OnExec(func(cmdLine string, result *shell.ExecResult) {
		if result.Code != 0 {
			m.onCommandFailed(ctx, cmdLine, result)
		}
	})

	// Hook 2: watch file changes under /workspace
	m.watcher = m.v.Watch("/workspace", grasp.EventAll)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			select {
			case ev, ok := <-m.watcher.Events():
				if !ok {
					return
				}
				m.onFileChanged(ctx, ev)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (m *agentMonitor) stop() {
	if m.cancel != nil {
		m.cancel()
	}
	if m.watcher != nil {
		_ = m.watcher.Close()
	}
	m.wg.Wait()
}

func (m *agentMonitor) onCommandFailed(ctx context.Context, cmdLine string, result *shell.ExecResult) {
	prompt := fmt.Sprintf(
		"The user just ran a command that failed.\n\nCommand: %s\nExit code: %d\nOutput:\n```\n%s```\n\n"+
			"Please briefly explain what went wrong and suggest a fix. Be concise (2-3 sentences max).",
		cmdLine, result.Code, result.Output,
	)
	m.agentRespond(ctx, prompt)
}

func (m *agentMonitor) onFileChanged(ctx context.Context, ev grasp.WatchEvent) {
	prompt := fmt.Sprintf(
		"A file change was detected in the virtual filesystem.\n\n"+
			"Event: %s\nPath: %s\nTime: %s\n\n"+
			"Briefly acknowledge this change. If the file is interesting (like notes or config), "+
			"offer to help with it. Be concise (1-2 sentences).",
		ev.Type, ev.Path, ev.Time.Format("15:04:05"),
	)
	m.agentRespond(ctx, prompt)
}

func (m *agentMonitor) agentRespond(ctx context.Context, trigger string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = append(m.messages, anthropic.NewUserMessage(anthropic.NewTextBlock(trigger)))

	shellTool := anthropic.ToolParam{
		Name:        "shell",
		Description: anthropic.String("Execute a shell command to inspect the virtual filesystem. Available: ls, cat, grep, find, head, tail, stat, etc."),
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

	systemPrompt := `You are a helpful shell assistant that monitors the user's activity in a grasp virtual filesystem. 
You observe command failures and file changes, then offer concise, actionable help.
When you need more context, use the shell tool to inspect files or directories.
Keep your responses brief — the user is in the middle of working.`

	fmt.Printf("\n\033[1;33m🤖 Agent:\033[0m ")

	for i := 0; i < 5; i++ {
		resp, err := m.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeSonnet4_5_20250929,
			MaxTokens: 1024,
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  m.messages,
			Tools:     []anthropic.ToolUnionParam{{OfTool: &shellTool}},
		})
		if err != nil {
			fmt.Printf("(agent error: %v)\n", err)
			return
		}

		m.messages = append(m.messages, resp.ToParam())

		hasToolUse := false
		var toolResults []anthropic.ContentBlockParamUnion

		for _, block := range resp.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				fmt.Print(b.Text)
			case anthropic.ToolUseBlock:
				hasToolUse = true
				var input struct{ Command string }
				if err := json.Unmarshal([]byte(b.JSON.Input.Raw()), &input); err != nil {
					log.Printf("unmarshal tool input: %v", err)
					continue
				}
				output := executeInAgentShell(m.v, input.Command)
				toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, output, false))
			}
		}

		if !hasToolUse {
			fmt.Println()
			break
		}
		m.messages = append(m.messages, anthropic.NewUserMessage(toolResults...))
	}

	// Trim conversation to last 20 messages to avoid unbounded growth
	if len(m.messages) > 20 {
		m.messages = m.messages[len(m.messages)-20:]
	}
}

func executeInAgentShell(v *grasp.VirtualOS, command string) string {
	agentSh := v.Shell("agent")
	result := agentSh.Execute(context.Background(), command)
	if result.Code != 0 && result.Output == "" {
		return fmt.Sprintf("exit code: %d", result.Code)
	}
	return result.Output
}

// ─── Setup helpers ───

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

func seedWorkspace(v *grasp.VirtualOS) {
	ctx := context.Background()
	v.Write(ctx, "/workspace/README.md", strings.NewReader(`# My Project
A demo workspace for the agent-monitor example.

## TODO
- Set up CI pipeline
- Write unit tests
- Add logging
`))
	v.Write(ctx, "/workspace/config.json", strings.NewReader(`{
  "app": "demo",
  "version": "0.1.0",
  "debug": true
}
`))
	v.Write(ctx, "/workspace/main.go", strings.NewReader(`package main

import "fmt"

func main() {
	fmt.Println("Hello from the workspace!")
}
`))
}
