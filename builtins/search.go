package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

func builtinSearch(v *shellfish.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`search — cross-mount search
Usage: search <query> [--scope <path>] [--max N]
       grep <pattern> [FILE]... (reads from stdin when no file specified)
`)), nil
		}

		if len(args) == 0 && stdin == nil {
			return nil, fmt.Errorf("search: missing query")
		}

		if stdin != nil && len(args) == 0 {
			data, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("grep: read error: %w", err)
			}
			return io.NopCloser(strings.NewReader(string(data))), nil
		}

		if len(args) > 0 && stdin != nil {
			data, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("grep: read error: %w", err)
			}
			pattern := args[0]
			lines := strings.Split(string(data), "\n")
			var matched []string
			for _, line := range lines {
				if strings.Contains(line, pattern) {
					matched = append(matched, line)
				}
			}
			return io.NopCloser(strings.NewReader(strings.Join(matched, "\n") + "\n")), nil
		}

		if len(args) == 0 {
			return nil, fmt.Errorf("search: missing query")
		}
		cwd := shellfish.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}
		opts := shellfish.SearchOpts{MaxResults: 20}
		query := args[0]
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--scope":
				if i+1 < len(args) {
					i++
					opts.Scope = resolvePath(cwd, args[i])
				}
			case "--max":
				if i+1 < len(args) {
					i++
					fmt.Sscanf(args[i], "%d", &opts.MaxResults)
				}
			}
		}
		results, err := v.Search(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
		if len(results) == 0 {
			return io.NopCloser(strings.NewReader("no results\n")), nil
		}
		var buf strings.Builder
		for _, r := range results {
			if r.Snippet != "" {
				fmt.Fprintf(&buf, "%s  %s\n", r.Entry.Path, r.Snippet)
			} else {
				fmt.Fprintf(&buf, "%s\n", r.Entry.Path)
			}
		}
		return io.NopCloser(strings.NewReader(buf.String())), nil
	}
}
