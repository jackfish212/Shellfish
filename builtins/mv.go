package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

func builtinMv(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("mv — move (rename) files\nUsage: mv <source> <dest>\n")), nil
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("mv: missing operand")
		}
		cwd := grasp.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}
		src := resolvePath(cwd, args[0])
		dst := resolvePath(cwd, args[1])
		if err := v.Rename(ctx, src, dst); err != nil {
			return nil, fmt.Errorf("mv: %w", err)
		}
		return io.NopCloser(strings.NewReader("")), nil
	}
}
