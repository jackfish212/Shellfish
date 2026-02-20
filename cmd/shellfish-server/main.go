// shellfish-server exposes a Shellfish VirtualOS as an MCP server over stdio.
//
// Usage:
//
//	shellfish-server [flags]
//
// Flags:
//
//	--mount PATH:SOURCE   Mount a filesystem (repeatable)
//	                      SOURCE formats:
//	                        ./dir           LocalFS (host directory)
//	                        sqlite:file.db  SQLiteFS (SQLite database)
//	                        memfs           MemFS (in-memory)
//	--user  NAME          Shell user name (default: "agent")
//	--debug               Enable debug logging to stderr
//	--version             Show version and exit
//
// Example:
//
//	shellfish-server --mount /data:./workspace --mount /memory:sqlite:memory.db
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/builtins"
	"github.com/jackfish212/shellfish/mcpserver"
	"github.com/jackfish212/shellfish/mounts"
)

// mountFlags collects repeatable --mount flags.
type mountFlags []string

func (m *mountFlags) String() string { return strings.Join(*m, ", ") }
func (m *mountFlags) Set(value string) error {
	*m = append(*m, value)
	return nil
}

func main() {
	var mntFlags mountFlags
	user := flag.String("user", "agent", "Shell user name")
	showVersion := flag.Bool("version", false, "Show version and exit")
	debug := flag.Bool("debug", false, "Enable debug logging to stderr")
	flag.Var(&mntFlags, "mount", "Mount specification PATH:SOURCE (repeatable)")
	flag.Parse()

	if *showVersion {
		info := shellfish.GetVersionInfo()
		fmt.Fprintf(os.Stdout, "shellfish-server %s (%s, %s)\n", info.Version, info.GoVersion, info.Platform)
		os.Exit(0)
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	v := shellfish.New()
	rootFS, err := shellfish.Configure(v)
	if err != nil {
		slog.Error("failed to configure VirtualOS", "error", err)
		os.Exit(1)
	}
	builtins.RegisterBuiltinsOnFS(v, rootFS)

	for _, spec := range mntFlags {
		if err := mountFromSpec(v, spec); err != nil {
			slog.Error("mount failed", "spec", spec, "error", err)
			os.Exit(1)
		}
		slog.Info("mounted", "spec", spec)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	srv := mcpserver.New(v, *user)
	if err := srv.Run(ctx, os.Stdin, os.Stdout); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// mountFromSpec parses "PATH:SOURCE" and mounts the appropriate provider.
//
// Supported SOURCE formats:
//
//	memfs            → in-memory MemFS
//	sqlite:file.db   → SQLiteFS backed by file.db
//	./dir or /abs    → LocalFS pointing at a host directory
func mountFromSpec(v *shellfish.VirtualOS, spec string) error {
	idx := strings.Index(spec, ":")
	if idx < 1 {
		return fmt.Errorf("invalid mount spec %q (expected PATH:SOURCE)", spec)
	}
	mountPath := spec[:idx]
	source := spec[idx+1:]

	// Ensure mount path starts with /
	if !strings.HasPrefix(mountPath, "/") {
		mountPath = "/" + mountPath
	}

	switch {
	case source == "memfs":
		return v.Mount(mountPath, mounts.NewMemFS(shellfish.PermRW))

	case strings.HasPrefix(source, "sqlite:"):
		dbPath := strings.TrimPrefix(source, "sqlite:")
		fs, err := mounts.NewSQLiteFS(dbPath, shellfish.PermRW)
		if err != nil {
			return fmt.Errorf("SQLiteFS %q: %w", dbPath, err)
		}
		return v.Mount(mountPath, fs)

	default:
		return v.Mount(mountPath, mounts.NewLocalFS(source, shellfish.PermRW))
	}
}
