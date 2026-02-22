// Example: Using grasp with Anthropic SDK
//
// This example demonstrates how to give Claude a shell interface
// through the Anthropic Go SDK. Claude can execute shell commands
// to interact with a virtual filesystem.
//
// Run: go run ./examples/anthropic
//
// Configuration: Copy .env.example to .env and fill in your credentials
//   - ANTHROPIC_BASE_URL: API base URL (e.g., https://open.bigmodel.cn/api/anthropic)
//   - ANTHROPIC_AUTH_TOKEN: Authentication token
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

func main() {
	// Parse command line flags
	interactive := flag.Bool("i", false, "Run in interactive mode")
	flag.Parse()

	// Load .env file from the example directory
	envPath := filepath.Join(".", ".env")
	if err := godotenv.Load(envPath); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
		log.Println("Using environment variables instead.")
	}
	// Initialize grasp VirtualOS
	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		panic(err)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	// Mount an in-memory filesystem for the project
	memFS := mounts.NewMemFS(grasp.PermRW)
	if err := v.Mount("/project", memFS); err != nil {
		panic(fmt.Errorf("failed to mount /project: %w", err))
	}

	// Mount a local filesystem for persistent data storage
	localFS := mounts.NewLocalFS(filepath.Join(".", "localfs-data"), grasp.PermRW)
	if err := v.Mount("/data", localFS); err != nil {
		panic(fmt.Errorf("failed to mount /data: %w", err))
	}

	// Create virtual project files
	setupVirtualProject(v)

	// Initialize Anthropic client
	// Supports ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN for custom endpoints
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	authToken := os.Getenv("ANTHROPIC_AUTH_TOKEN")

	var client anthropic.Client
	if baseURL != "" && authToken != "" {
		// Custom endpoint (e.g., Zhipu AI, other Anthropic-compatible APIs)
		client = anthropic.NewClient(
			option.WithBaseURL(baseURL),
			option.WithAPIKey(authToken),
		)
	} else {
		// Default Anthropic API (uses ANTHROPIC_API_KEY)
		client = anthropic.NewClient()
	}

	// Define the shell tool schema
	shellTool := anthropic.ToolParam{
		Name:        "shell",
		Description: anthropic.String("Execute a shell command in the virtual filesystem. Available commands: ls, cat, read, write, stat, grep, find, head, tail, mkdir, rm, mv, echo, pwd, cd. Use pipes (|) and redirects (>, >>) to chain commands."),
		InputSchema: anthropic.ToolInputSchemaParam{
			Type: constant.ValueOf[constant.Object](),
			Properties: map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute (e.g., 'ls /project', 'cat README.md', 'grep TODO *.go')",
				},
			},
			Required: []string{"command"},
		},
	}

	ctx := context.Background()

	// Start conversation
	fmt.Println("grasp + Anthropic SDK Example")
	fmt.Println("=================================")
	fmt.Println("Filesystems mounted:")
	fmt.Println("  /project - in-memory filesystem (memfs)")
	fmt.Println("  /data    - local disk filesystem (localfs-data/)")
	fmt.Println()

	if *interactive {
		runInteractiveMode(ctx, v, client, shellTool)
	} else {
		runDefaultTask(ctx, v, client, shellTool)
	}
}

func runInteractiveMode(ctx context.Context, v *grasp.VirtualOS, client anthropic.Client, shellTool anthropic.ToolParam) {
	fmt.Println("Interactive Mode")
	fmt.Println("=============== ")
	fmt.Println("Type your message and press Enter to chat with the agent.")
	fmt.Println("The agent has access to /project and /data filesystems.")
	fmt.Println("Press Ctrl+C or type 'exit' to quit.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	messages := []anthropic.MessageParam{}

	systemPrompt := "You are an AI assistant with access to a virtual filesystem through shell commands. Use the 'shell' tool to explore files, read content, and interact with the filesystem. You have access to two filesystems: /project (in-memory, contains sample project) and /data (local disk, for persistent storage). Be helpful and complete tasks step by step."

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

		// Add user message
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(input)))

		// Process with agent
		messages = processAgentLoop(ctx, v, client, shellTool, messages, systemPrompt)
	}
}

func runDefaultTask(ctx context.Context, v *grasp.VirtualOS, client anthropic.Client, shellTool anthropic.ToolParam) {
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

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(task)),
	}

	systemPrompt := "You are an AI assistant with access to a virtual filesystem through shell commands. Use the 'shell' tool to explore files, read content, and interact with the filesystem. Be thorough and complete the given task step by step."
	messages = processAgentLoop(ctx, v, client, shellTool, messages, systemPrompt)

	fmt.Println("\n[Done]")

	// Show the created files in /data
	fmt.Println("\n=== Files in /data ===")
	result := executeShell(v, "ls -la /data")
	fmt.Println(result)

	fmt.Println("\n=== /data/todos.txt ===")
	result = executeShell(v, "cat /data/todos.txt")
	fmt.Println(result)

	fmt.Println("\n=== /data/project-info.json ===")
	result = executeShell(v, "cat /data/project-info.json")
	fmt.Println(result)
}

func processAgentLoop(ctx context.Context, v *grasp.VirtualOS, client anthropic.Client, shellTool anthropic.ToolParam, messages []anthropic.MessageParam, systemPrompt string) []anthropic.MessageParam {
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

		// Process response
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
					// Parse input
					var input struct {
						Command string `json:"command"`
					}
					inputJSON := b.JSON.Input.Raw()
					if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
						toolResults = append(toolResults, anthropic.NewToolResultBlock(b.ID, fmt.Sprintf("Error parsing input: %v", err), true))
						continue
					}

					fmt.Printf("\n[shell] %s\n", input.Command)

					// Execute shell command
					result := executeShell(v, input.Command)
					fmt.Printf("[result]\n%s\n", truncate(result, 1000))

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

func setupVirtualProject(v *grasp.VirtualOS) {
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

func executeShell(v *grasp.VirtualOS, command string) string {
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
