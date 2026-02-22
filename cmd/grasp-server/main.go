// grasp-server exposes a grasp VirtualOS as an MCP server over stdio.
//
// Usage:
//
//	grasp-server [flags]
//
// Flags:
//
//	--mount PATH:SOURCE   Mount a filesystem (repeatable)
//	                      SOURCE formats:
//	                        ./dir           LocalFS (host directory)
//	                        memfs           MemFS (in-memory)
//	--user  NAME          Shell user name (default: "agent")
//	--debug               Enable debug logging to stderr
//	--version             Show version and exit
//
// Example:
//
//	grasp-server --mount /data:./workspace --mount /memory:memfs
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/builtins"
	"github.com/jackfish212/grasp/mcpserver"
	"github.com/jackfish212/grasp/mounts"
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
		info := grasp.GetVersionInfo()
		_, _ = fmt.Fprintf(os.Stdout, "grasp-server %s (%s, %s)\n", info.Version, info.GoVersion, info.Platform)
		os.Exit(0)
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	v := grasp.New()
	rootFS, err := grasp.Configure(v)
	if err != nil {
		slog.Error("failed to configure VirtualOS", "error", err)
		os.Exit(1)
	}
	if err := builtins.RegisterBuiltinsOnFS(v, rootFS); err != nil {
		slog.Error("failed to register builtins", "error", err)
		os.Exit(1)
	}

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
//	./dir or /abs    → LocalFS pointing at a host directory
func mountFromSpec(v *grasp.VirtualOS, spec string) error {
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
		return v.Mount(mountPath, mounts.NewMemFS(grasp.PermRW))

	default:
		return v.Mount(mountPath, mounts.NewLocalFS(source, grasp.PermRW))
	}
}
