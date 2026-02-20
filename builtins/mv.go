package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	afs "github.com/agentfs/afs"
	"github.com/agentfs/afs/mounts"
)

func builtinMv(v *afs.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("mv — move (rename) files\nUsage: mv <source> <dest>\n")), nil
		}
		if len(args) < 2 {
			return nil, fmt.Errorf("mv: missing operand")
		}
		cwd := afs.Env(ctx, "PWD")
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
