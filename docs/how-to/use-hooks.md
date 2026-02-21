# Reactive Agents with Hooks

GRASP provides two mechanisms for building reactive agents that respond to filesystem changes and command execution:

1. **Watch** — inotify-style filesystem event notifications
2. **OnExec** — post-execution hooks for shell commands

These enable agents to observe user activity and provide contextual assistance.

## Filesystem Watch

Use `Watch()` to monitor filesystem changes:

```go
import "github.com/jackfish212/grasp"

// Create watcher for specific path and event types
watcher := v.Watch("/workspace", grasp.EventAll)

// Available event types:
//   EventCreate - file/directory created
//   EventWrite  - file written
//   EventRemove - file/directory removed
//   EventMkdir  - directory created
//   EventRename - file/directory renamed
//   EventAll    - all events
```

### Consuming Events

```go
for {
    select {
    case event := <-watcher.Events:
        fmt.Printf("Event: %s on %s\n", event.Type, event.Path)

        switch event.Type {
        case grasp.EventWrite:
            // File was modified
            content, _ := sh.Execute(ctx, "cat "+event.Path)
            analyzeChange(content)
        case grasp.EventCreate:
            // New file created
            fmt.Printf("New file: %s\n", event.Path)
        }

    case err := <-watcher.Errors:
        log.Printf("Watch error: %v", err)
    }
}
```

### Combining with Agent

```go
func runWatcherAgent(ctx context.Context, v *grasp.VirtualOS, client *anthropic.Client) {
    watcher := v.Watch("/workspace", grasp.EventWrite)

    for {
        select {
        case event := <-watcher.Events:
            // Read changed file
            content, _ := v.Open(ctx, event.Path)

            // Ask agent to analyze the change
            response, _ := client.Messages.New(ctx, anthropic.MessageNewParams{
                Model: anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
                MaxTokens: anthropic.F(int64(1024)),
                Messages: anthropic.F([]anthropic.MessageParam{
                    anthropic.NewUserMessage(anthropic.NewTextBlock(
                        fmt.Sprintf("The file %s was just modified. Content:\n\n%s\n\nSuggest improvements.",
                            event.Path, content)),
                    ),
                }),
            })
            fmt.Println(response.Content[0].Text)

        case <-ctx.Done():
            watcher.Close()
            return
        }
    }
}
```

## Execution Hooks

Use `OnExec()` to hook into command execution:

```go
sh := v.Shell("user")

// Register hook - called after every command
sh.OnExec(func(cmdLine string, result *shell.ExecResult) {
    log.Printf("Command: %s (exit: %d)", cmdLine, result.Code)

    if result.Code != 0 {
        // Command failed - offer assistance
        suggestFix(cmdLine, result.Output)
    }
})
```

### Hook Structure

```go
type ExecHook func(cmdLine string, result *ExecResult)

type ExecResult struct {
    Output string  // Combined stdout + stderr
    Code   int     // Exit code (0 = success)
}
```

### Auto-Help on Failure

```go
func setupAutoHelp(sh *grasp.Shell, client *anthropic.Client) {
    sh.OnExec(func(cmdLine string, result *shell.ExecResult) {
        if result.Code == 0 {
            return // Success, no action needed
        }

        // Command failed - ask agent for help
        prompt := fmt.Sprintf(`A shell command failed. Help the user fix it.

Command: %s
Exit code: %d
Output:
%s

Suggest a fix or alternative command.`,
            cmdLine, result.Code, result.Output)

        resp, err := client.Messages.New(context.Background(), anthropic.MessageNewParams{
            Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
            MaxTokens: anthropic.F(int64(1024)),
            Messages: anthropic.F([]anthropic.MessageParam{
                anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
            }),
        })

        if err == nil {
            fmt.Printf("\n💡 Suggestion: %s\n\n", resp.Content[0].Text)
        }
    })
}
```

## Combined Example: Agent Monitor

This example demonstrates both hooks working together:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/jackfish212/grasp"
    "github.com/jackfish212/grasp/builtins"
    "github.com/jackfish212/grasp/mounts"
    "github.com/jackfish212/grasp/shell"
)

type MonitorAgent struct {
    v      *grasp.VirtualOS
    sh     *grasp.Shell
    client *anthropic.Client
}

func main() {
    // Setup
    v := grasp.New()
    rootFS, _ := grasp.Configure(v)
    builtins.RegisterBuiltinsOnFS(v, rootFS)
    v.Mount("/workspace", mounts.NewLocalFS(".", grasp.PermRW))

    agent := &MonitorAgent{
        v:      v,
        sh:     v.Shell("user"),
        client: anthropic.NewClient(),
    }

    // Start hooks
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go agent.watchFiles(ctx)
    agent.setupExecHook()

    // Simulate user activity
    agent.runUserSession(ctx)
}

func (a *MonitorAgent) watchFiles(ctx context.Context) {
    watcher := a.v.Watch("/workspace", grasp.EventWrite|grasp.EventCreate)

    for {
        select {
        case event := <-watcher.Events:
            a.handleFileEvent(ctx, event)
        case err := <-watcher.Errors:
            log.Printf("Watch error: %v", err)
        case <-ctx.Done():
            watcher.Close()
            return
        }
    }
}

func (a *MonitorAgent) handleFileEvent(ctx context.Context, event grasp.WatchEvent) {
    fmt.Printf("\n📁 File change detected: %s (%s)\n", event.Path, event.Type)

    // Read file content
    file, err := a.v.Open(ctx, event.Path)
    if err != nil {
        return
    }
    defer file.Close()

    // Analyze with LLM
    prompt := fmt.Sprintf(`A file was just modified in the workspace:

Path: %s
Event: %s

Briefly analyze this change and note any potential issues.`,
        event.Path, event.Type)

    resp, _ := a.client.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
        MaxTokens: anthropic.F(int64(512)),
        Messages: anthropic.F([]anthropic.MessageParam{
            anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
        }),
    })

    fmt.Printf("💡 Analysis: %s\n", resp.Content[0].Text)
}

func (a *MonitorAgent) setupExecHook() {
    a.sh.OnExec(func(cmdLine string, result *shell.ExecResult) {
        if result.Code == 0 {
            return // Success
        }

        fmt.Printf("\n❌ Command failed: %s (exit %d)\n", cmdLine, result.Code)

        // Ask agent for help
        prompt := fmt.Sprintf(`A command failed:

Command: %s
Exit code: %d
Error output:
%s

Briefly suggest how to fix this.`,
            cmdLine, result.Code, result.Output)

        resp, err := a.client.Messages.New(context.Background(), anthropic.MessageNewParams{
            Model:     anthropic.F(anthropic.ModelClaude3_5SonnetLatest),
            MaxTokens: anthropic.F(int64(512)),
            Messages: anthropic.F([]anthropic.MessageParam{
                anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
            }),
        })

        if err == nil {
            fmt.Printf("💡 Fix suggestion: %s\n", resp.Content[0].Text)
        }
    })
}

func (a *MonitorAgent) runUserSession(ctx context.Context) {
    fmt.Println("Agent Monitor Active")
    fmt.Println("Commands will be monitored for errors.")
    fmt.Println("File changes will trigger analysis.")
    fmt.Println()

    // Handle signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    <-sigChan
    fmt.Println("\nShutting down...")
}
```

## Best Practices

### Debouncing Events

File writes can generate multiple events. Debounce to avoid overwhelming the agent:

```go
type Debouncer struct {
    timers map[string]*time.Timer
    mu     sync.Mutex
}

func (d *Debouncer) Debounce(key string, duration time.Duration, fn func()) {
    d.mu.Lock()
    defer d.mu.Unlock()

    if timer, exists := d.timers[key]; exists {
        timer.Stop()
    }

    d.timers[key] = time.AfterFunc(duration, fn)
}

// Usage
debouncer := &Debouncer{timers: make(map[string]*time.Timer)}

watcher := v.Watch("/workspace", grasp.EventWrite)
for event := range watcher.Events {
    debouncer.Debounce(event.Path, 500*time.Millisecond, func() {
        analyzeFile(event.Path)
    })
}
```

### Filtering Events

Only watch relevant paths:

```go
watcher := v.Watch("/workspace/src", grasp.EventWrite)

// Or filter in the handler
for event := range watcher.Events {
    if strings.HasSuffix(event.Path, ".go") {
        analyzeGoFile(event.Path)
    }
}
```

### Error Recovery

Always handle watcher errors:

```go
for {
    select {
    case event := <-watcher.Events:
        // Handle event
    case err := <-watcher.Errors:
        if errors.Is(err, net.ErrClosed) {
            return // Watcher closed
        }
        log.Printf("Watch error: %v", err)
    }
}
```
