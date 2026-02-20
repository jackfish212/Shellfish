# Getting Started

This tutorial walks you through setting up Shellfish, mounting data sources, and running shell commands. By the end, you'll have a working virtual filesystem with local files, in-memory tools, and a shell interface.

## Prerequisites

- Go 1.24+
- A Go module project

## Install

```bash
go get github.com/jackfish212/shellfish@latest
```

## Minimal Example

Create a `main.go`:

```go
package main

import (
    "context"
    "fmt"

    "github.com/jackfish212/shellfish"
    "github.com/jackfish212/shellfish/builtins"
    "github.com/jackfish212/shellfish/mounts"
)

func main() {
    // 1. Create the virtual OS
    v := shellfish.New()

    // 2. Configure standard filesystem layout (/bin, /usr, /etc, /proc, ...)
    rootFS, err := shellfish.Configure(v)
    if err != nil {
        panic(err)
    }

    // 3. Register built-in commands (ls, cat, grep, search, ...)
    builtins.RegisterBuiltinsOnFS(v, rootFS)

    // 4. Mount a local directory
    v.Mount("/data", mounts.NewLocalFS(".", shellfish.PermRW))

    // 5. Create a shell and run commands
    sh := v.Shell("agent")
    ctx := context.Background()

    result := sh.Execute(ctx, "ls /data")
    fmt.Println(result.Output)

    result = sh.Execute(ctx, "cat /proc/version")
    fmt.Println(result.Output)
}
```

Run it:

```bash
go run main.go
```

You'll see a listing of your current directory under `/data` and the Shellfish version string.

## Adding In-Memory Files

You can add files directly to the root MemFS:

```go
rootFS.AddFile("etc/motd", []byte("Welcome to Shellfish\n"), shellfish.PermRO)

result := sh.Execute(ctx, "cat /etc/motd")
// Output: Welcome to Shellfish
```

## Registering Custom Commands

Register Go functions as executable commands:

```go
rootFS.AddExecFunc("usr/bin/greet", func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
    name := "world"
    if len(args) > 0 {
        name = args[0]
    }
    msg := fmt.Sprintf("Hello, %s!\n", name)
    return io.NopCloser(strings.NewReader(msg)), nil
}, mounts.FuncMeta{
    Description: "Greet someone",
    Usage:       "greet [name]",
})

result := sh.Execute(ctx, "greet Agent")
// Output: Hello, Agent!
```

Because `greet` is at `/usr/bin/greet` and `/usr/bin` is on PATH, the shell finds it automatically.

## Using Pipes and Composition

Shell commands compose through pipes:

```go
sh.Execute(ctx, "ls /data | head -5")
sh.Execute(ctx, "cat /data/main.go | grep import")
sh.Execute(ctx, "search TODO --scope /data | head -3")
```

Use redirection to write output to files:

```go
sh.Execute(ctx, "echo 'task done' > /tmp/log.md")
sh.Execute(ctx, "cat /tmp/log.md")
// Output: task done
```

Logical operators for conditional execution:

```go
sh.Execute(ctx, "mkdir /tmp/work && echo 'created' || echo 'failed'")
```

## Mounting SQLite for Persistence

```go
sqlFS, err := mounts.NewSQLiteFS("/var/data/memory.db", shellfish.PermRW)
if err != nil {
    panic(err)
}
v.Mount("/memory", sqlFS)

sh.Execute(ctx, "echo 'remember this' | write /memory/notes.md")
// Data persists across restarts
```

## Multiple Shells

Each `Shell` instance is independent — different working directories, different environment variables, different history. Create one per agent session:

```go
sh1 := v.Shell("alice")
sh2 := v.Shell("bob")

sh1.Execute(ctx, "cd /data")
sh2.Execute(ctx, "cd /memory")

// sh1.Cwd() == "/data"
// sh2.Cwd() == "/memory"
```

## Inspecting the System

```go
// List all mount points
sh.Execute(ctx, "mount")

// System information
sh.Execute(ctx, "uname -a")

// Find a command
sh.Execute(ctx, "which grep")

// Environment variables
sh.Execute(ctx, "env")
```

## Next Steps

- Read [Architecture](../explanation/architecture.md) to understand how VirtualOS, MountTable, and Shell work together.
- Read [Provider Model](../explanation/provider-model.md) to learn how capability-based interfaces work.
- Read [Create a Custom Provider](../how-to/create-provider.md) for a step-by-step guide to building your own provider.
- Read [Shell as Interface](../explanation/shell-as-interface.md) to understand why shell commands are the optimal agent interface.
