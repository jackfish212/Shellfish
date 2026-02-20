package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

func builtinMkdir(v *shellfish.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("mkdir — create directories\nUsage: mkdir [-p] <path>...\n")), nil
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("mkdir: missing operand")
		}
		cwd := shellfish.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		var out strings.Builder
		for _, arg := range args {
			if arg == "-p" {
				continue
			}
			target := resolvePath(cwd, arg)
			if err := v.Mkdir(ctx, target, shellfish.PermRWX); err != nil {
				fmt.Fprintf(&out, "mkdir: %v\n", err)
				continue
			}
		}
		return io.NopCloser(strings.NewReader(out.String())), nil
	}
}
