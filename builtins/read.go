package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

func builtinRead(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`read — read file content
Usage: read <path>

cat — concatenate files and print to stdout
Usage: cat [FILE]...
       cat (read from stdin when no file specified)
`)), nil
		}

		if len(args) == 0 {
			if stdin == nil {
				return nil, fmt.Errorf("read: missing path")
			}
			data, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("read: %w", err)
			}
			return io.NopCloser(strings.NewReader(string(data))), nil
		}

		cwd := grasp.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		var results []string
		for _, arg := range args {
			target := resolvePath(cwd, arg)
			rc, err := v.Open(ctx, target)
			if err != nil {
				return nil, fmt.Errorf("read: %w", err)
			}
			defer func() { _ = rc.Close() }()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("read: %w", err)
			}
			results = append(results, string(data))
		}
		return io.NopCloser(strings.NewReader(strings.Join(results, ""))), nil
	}
}
