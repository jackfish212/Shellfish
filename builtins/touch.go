package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

func builtinTouch(v *shellfish.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("touch — update file timestamps or create empty files\nUsage: touch <file>...\n")), nil
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("touch: missing operand")
		}
		cwd := shellfish.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		var out strings.Builder
		for _, arg := range args {
			target := resolvePath(cwd, arg)
			if err := v.Touch(ctx, target); err != nil {
				fmt.Fprintf(&out, "touch: %v\n", err)
			}
		}
		return io.NopCloser(strings.NewReader(out.String())), nil
	}
}
