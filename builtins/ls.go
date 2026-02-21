package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

func builtinLs(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("ls — list directory entries\nUsage: ls [path...]\n")), nil
		}

		showLong, showAll, filteredArgs := parseLsFlags(args)

		cwd := grasp.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		targets := []string{cwd}
		if len(filteredArgs) > 0 {
			targets = make([]string, len(filteredArgs))
			for i, arg := range filteredArgs {
				targets[i] = resolvePath(cwd, arg)
			}
		}

		var buf strings.Builder
		for i, target := range targets {
			if len(targets) > 1 {
				if i > 0 {
					buf.WriteByte('\n')
				}
				buf.WriteString(target)
				buf.WriteString(":\n")
			}
			entries, err := v.List(ctx, target, grasp.ListOpts{})
			if err != nil {
				if entry, statErr := v.Stat(ctx, target); statErr == nil {
					entries = []grasp.Entry{*entry}
				} else {
					return nil, fmt.Errorf("ls: %w", err)
				}
			}
			if len(entries) == 0 {
				if entry, statErr := v.Stat(ctx, target); statErr == nil {
					entries = []grasp.Entry{*entry}
				}
			}
			var filteredEntries []grasp.Entry
			for _, e := range entries {
				if !showAll && strings.HasPrefix(e.Name, ".") {
					continue
				}
				filteredEntries = append(filteredEntries, e)
			}
			for j, e := range filteredEntries {
				if showLong {
					buf.WriteString(e.String())
					buf.WriteByte('\n')
				} else {
					buf.WriteString(e.Name)
					if e.IsDir {
						buf.WriteByte('/')
					}
					if j < len(filteredEntries)-1 {
						buf.WriteByte(' ')
					}
				}
			}
		}
		return io.NopCloser(strings.NewReader(buf.String())), nil
	}
}
