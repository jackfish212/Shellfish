// Example: Using Shellfish with OpenAI Agents SDK
//
// This example demonstrates how to give an AI agent a shell interface
// through the openai-agents-go SDK. The agent can execute shell commands
// to interact with a virtual filesystem.
//
// Run: go run ./examples/openai
//
// Configuration: Copy .env.example to .env and fill in your credentials
//   - OPENAI_API_KEY: Your OpenAI API key
//   - OPENAI_BASE_URL: Optional, for custom endpoints (e.g., Azure OpenAI)
//   - OPENAI_MODEL: Optional, defaults to gpt-4o
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/builtins"
	"github.com/jackfish212/shellfish/mounts"

	"github.com/nlpodyssey/openai-agents-go/agents"
	"github.com/openai/openai-go/v2/packages/param"
)

// ConsoleHooks implements RunHooks to display execution progress
type ConsoleHooks struct {
	agents.NoOpRunHooks
}

func (h ConsoleHooks) OnToolStart(ctx context.Context, agent *agents.Agent, tool agents.Tool) error {
	fmt.Printf("[tool] %s\n", tool.ToolName())
	return nil
}

func (h ConsoleHooks) OnToolEnd(ctx context.Context, agent *agents.Agent, tool agents.Tool, result any) error {
	if result != nil {
		output := fmt.Sprintf("%v", result)
		if len(output) > 200 {
			output = output[:200] + "... (truncated)"
		}
		fmt.Printf("[result] %s\n", output)
	}
	return nil
}

func main() {
	// Parse command line flags
	interactive := flag.Bool("i", false, "Run in interactive mode")
	flag.Parse()

	// Suppress tracing ERROR logs (use a very high level to hide all SDK internal logs)
	agents.SetLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError + 4, // Higher than ERROR to suppress all SDK logs
	})))

	// Load .env file from the example directory
	envPath := filepath.Join(".", ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
		log.Println("Using environment variables instead.")
	}

	// Initialize Shellfish VirtualOS
	v := shellfish.New()
	rootFS, err := shellfish.Configure(v)
	if err != nil {
		panic(err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// Mount an in-memory filesystem for the project
	memFS := mounts.NewMemFS(shellfish.PermRW)
	if err := v.Mount("/project", memFS); err != nil {
		panic(fmt.Errorf("failed to mount /project: %w", err))
	}

	// Mount a local filesystem for persistent data storage
	localFS := mounts.NewLocalFS(filepath.Join(".", "localfs-data"), shellfish.PermRW)
	if err := v.Mount("/data", localFS); err != nil {
		panic(fmt.Errorf("failed to mount /data: %w", err))
	}

	// Create virtual project files
	setupVirtualProject(v)

	// Create the shell tool
	shellTool := createShellTool(v)

	// Get model from environment or use default
	model := getEnvOrDefault("OPENAI_MODEL", "gpt-4o")

	// Create the agent
	agent := agents.New("Shell Assistant").
		WithInstructions(`You are an AI assistant with access to a virtual filesystem through shell commands.
Use the 'shell' tool to explore files, read content, and interact with the filesystem.
You have access to two filesystems:
- /project (in-memory, contains sample project)
- /data (local disk, for persistent storage)

Available commands: ls, cat, read, write, stat, grep, find, head, tail, mkdir, rm, mv, echo, pwd, cd.
Use pipes (|) and redirects (>, >>) to chain commands.
Be helpful and complete tasks step by step.`).
		WithTools(shellTool).
		WithModel(model)

	// Create run config with custom provider if base URL is set
	baseURL := os.Getenv("OPENAI_BASE_URL")
	var runner agents.Runner

	// Console hooks to display execution progress
	consoleHooks := ConsoleHooks{}

	if baseURL != "" {
		// Use Chat Completions API instead of Responses API for better compatibility
		// with OpenAI-compatible providers (e.g., Zhipu AI, Azure OpenAI)
		provider := agents.NewOpenAIProvider(agents.OpenAIProviderParams{
			BaseURL:     param.NewOpt(baseURL),
			UseResponses: param.NewOpt(false),
		})
		runner = agents.Runner{
			Config: agents.RunConfig{
				ModelProvider:   provider,
				TracingDisabled: true, // Disable tracing for non-OpenAI endpoints
				Hooks:           consoleHooks,
			},
		}
		fmt.Printf("Using custom base URL: %s\n", baseURL)
	} else {
		runner = agents.Runner{
			Config: agents.RunConfig{
				Hooks: consoleHooks,
			},
		}
	}

	ctx := context.Background()

	// Start conversation
	fmt.Println("Shellfish + OpenAI Agents SDK Example")
	fmt.Println("======================================")
	fmt.Println("Filesystems mounted:")
	fmt.Println("  /project - in-memory filesystem (memfs)")
	fmt.Println("  /data    - local disk filesystem (localfs-data/)")
	fmt.Println()

	if *interactive {
		runInteractiveMode(ctx, agent, runner)
	} else {
		runDefaultTask(ctx, agent, runner)
	}
}

// ShellArgs defines the arguments for the shell tool
type ShellArgs struct {
	Command string `json:"command" description:"The shell command to execute (e.g., 'ls /project', 'cat README.md', 'grep TODO *.go')"`
}

// ShellResult defines the result of shell execution
type ShellResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

// createShellTool creates a function tool for shell command execution
func createShellTool(v *shellfish.VirtualOS) agents.FunctionTool {
	// Define the shell execution function
	execShell := func(ctx context.Context, args ShellArgs) (ShellResult, error) {
		sh := v.Shell("agent")
		result := sh.Execute(ctx, args.Command)

		output := result.Output
		if result.Code != 0 {
			if output != "" && !strings.HasSuffix(output, "\n") {
				output += "\n"
			}
			output += fmt.Sprintf("exit code: %d", result.Code)
		}

		return ShellResult{
			Output:   output,
			ExitCode: result.Code,
		}, nil
	}

	return agents.NewFunctionTool(
		"shell",
		"Execute a shell command in the virtual filesystem. Available commands: ls, cat, read, write, stat, grep, find, head, tail, mkdir, rm, mv, echo, pwd, cd. Use pipes (|) and redirects (>, >>) to chain commands.",
		execShell,
	)
}

func runInteractiveMode(ctx context.Context, agent *agents.Agent, runner agents.Runner) {
	fmt.Println("Interactive Mode")
	fmt.Println("=============== ")
	fmt.Println("Type your message and press Enter to chat with the agent.")
	fmt.Println("The agent has access to /project and /data filesystems.")
	fmt.Println("Press Ctrl+C or type 'exit' to quit.")
	fmt.Println()

	for {
		fmt.Print("You: ")
		var input string
		if _, err := fmt.Scanln(&input); err != nil {
			if err.Error() == "unexpected newline" {
				continue
			}
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

		// Run the agent with user input
		var result *agents.RunResult
		var err error
		if runner.Config.ModelProvider != nil {
			result, err = runner.Run(ctx, agent, input)
		} else {
			result, err = agents.Run(ctx, agent, input)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		// Print the agent's response
		fmt.Printf("\nAssistant: %s\n\n", result.FinalOutput)
	}
}

func runDefaultTask(ctx context.Context, agent *agents.Agent, runner agents.Runner) {
	// Default task: cross-filesystem task between memfs (/project) and localfs (/data)
	task := `You have access to TWO filesystems:
- /project (in-memory filesystem) - contains source code
- /data (local filesystem backed by disk) - for persistent storage

Complete the following cross-filesystem task:

1. Explore /project and understand the TaskManager project structure
2. Extract all TODO items from the code files in /project
3. Create a backup directory in /data/backups
4. Copy important files from /project to /data/backups/ (README.md, go.mod, config.json)
5. Create /data/todos.txt with a numbered list of all TODO items found
6. Create /data/project-info.json with:
   - Project name
   - Number of files
   - Total lines of code (estimate)
   - List of TODO items

Use shell commands to work across both filesystems. The /data filesystem persists to disk.`

	fmt.Printf("Task: %s\n\n", task)
	fmt.Println("Starting agent...")

	// Run the agent
	var result *agents.RunResult
	var err error
	if runner.Config.ModelProvider != nil {
		result, err = runner.Run(ctx, agent, task)
	} else {
		result, err = agents.Run(ctx, agent, task)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running agent: %v\n", err)
		return
	}

	fmt.Printf("\nAssistant: %s\n", result.FinalOutput)

	fmt.Println("\n[Done]")
}

func setupVirtualProject(v *shellfish.VirtualOS) {
	ctx := context.Background()

	// Create README.md
	readme := `# TaskManager

A simple task management library for Go applications.

## Features

- Create, update, and delete tasks
- Task priority levels (Low, Medium, High)
- Task status tracking (Pending, In Progress, Completed)
- Basic task filtering

## Installation

` + "```bash" + `
go get github.com/example/taskmanager
` + "```" + `

## Quick Start

` + "```go" + `
tm := taskmanager.New()
tm.AddTask("Write documentation", taskmanager.PriorityHigh)
tm.ListTasks()
` + "```" + `

## TODO

- [ ] Add due date support
- [ ] Implement task categories
- [ ] Add persistence layer
- [ ] Write more unit tests
`
	v.Write(ctx, "/project/README.md", strings.NewReader(readme))

	// Create main.go
	mainGo := `package main

import (
	"fmt"
	"github.com/example/taskmanager"
)

func main() {
	tm := taskmanager.New()

	// Add some tasks
	tm.AddTask("Learn Go", taskmanager.PriorityHigh)
	tm.AddTask("Build a CLI tool", taskmanager.PriorityMedium)
	tm.AddTask("Write tests", taskmanager.PriorityLow)

	// List all tasks
	tasks := tm.ListTasks()
	for _, t := range tasks {
		fmt.Printf("[%s] %s - %s\n", t.Status, t.Title, t.Priority)
	}

	// TODO: implement task completion
	// TODO: add command line argument parsing
}
`
	v.Write(ctx, "/project/main.go", strings.NewReader(mainGo))

	// Create task.go
	taskGo := `package taskmanager

import "time"

// Priority levels for tasks
type Priority int

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
)

// Status represents task status
type Status int

const (
	StatusPending Status = iota
	StatusInProgress
	StatusCompleted
)

// Task represents a single task
type Task struct {
	ID          int
	Title       string
	Description string
	Priority    Priority
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewTask creates a new task with default values
func NewTask(title string, priority Priority) *Task {
	now := time.Now()
	return &Task{
		Title:     title,
		Priority:  priority,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
`
	v.Write(ctx, "/project/task.go", strings.NewReader(taskGo))

	// Create manager.go
	managerGo := `package taskmanager

import (
	"errors"
	"sync"
)

// Manager handles task operations
type Manager struct {
	tasks  map[int]*Task
	nextID int
	mu     sync.RWMutex
}

// New creates a new task manager
func New() *Manager {
	return &Manager{
		tasks:  make(map[int]*Task),
		nextID: 1,
	}
}

// AddTask creates and adds a new task
func (m *Manager) AddTask(title string, priority Priority) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := NewTask(title, priority)
	task.ID = m.nextID
	m.tasks[task.ID] = task
	m.nextID++

	return task
}

// ListTasks returns all tasks
func (m *Manager) ListTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tasks = append(tasks, t)
	}
	return tasks
}

// UpdateStatus updates a task's status
func (m *Manager) UpdateStatus(id int, status Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, exists := m.tasks[id]
	if !exists {
		return errors.New("task not found")
	}

	task.Status = status
	task.UpdatedAt = time.Now()
	return nil
}

// Delete removes a task
func (m *Manager) Delete(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tasks[id]; !exists {
		return errors.New("task not found")
	}

	delete(m.tasks, id)
	return nil
}

// TODO: Add filtering by status
// TODO: Add filtering by priority
// TODO: Add task search functionality
`
	v.Write(ctx, "/project/manager.go", strings.NewReader(managerGo))

	// Create go.mod
	goMod := `module github.com/example/taskmanager

go 1.22

require (
	github.com/stretchr/testify v1.8.0
)
`
	v.Write(ctx, "/project/go.mod", strings.NewReader(goMod))

	// Create a config file
	config := `{
	"app_name": "TaskManager",
	"version": "0.1.0",
	"max_tasks": 1000,
	"storage": {
		"type": "memory",
		"backup_enabled": false
	},
	"logging": {
		"level": "info",
		"output": "stdout"
	}
}
`
	v.Write(ctx, "/project/config.json", strings.NewReader(config))
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
