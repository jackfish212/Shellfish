package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	afs "github.com/agentfs/afs"
	"github.com/agentfs/afs/mounts"
)

func builtinRm(v *afs.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("rm — remove files or directories\nUsage: rm [-r|-rf] <path>...\n")), nil
		}

		var paths []string
		for _, arg := range args {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			paths = append(paths, arg)
		}

		if len(paths) == 0 {
			return nil, fmt.Errorf("rm: missing operand")
		}

		cwd := afs.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		for _, p := range paths {
			target := resolvePath(cwd, p)
			if err := v.Remove(ctx, target); err != nil {
				return nil, fmt.Errorf("rm: %v", err)
			}
		}
		return io.NopCloser(strings.NewReader("")), nil
	}
}
