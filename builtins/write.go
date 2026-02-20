package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

func builtinWrite(v *shellfish.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("write — write content to file\nUsage: write <path> [content]\n")), nil
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("write: missing path")
		}
		cwd := shellfish.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}
		target := resolvePath(cwd, args[0])
		var r io.Reader
		if len(args) > 1 {
			r = strings.NewReader(strings.Join(args[1:], " "))
		} else if stdin != nil {
			r = stdin
		} else {
			return nil, fmt.Errorf("write: no content (provide inline or via pipe)")
		}
		if err := v.Write(ctx, target, r); err != nil {
			return nil, fmt.Errorf("write: %w", err)
		}
		return io.NopCloser(strings.NewReader(fmt.Sprintf("wrote: %s\n", target))), nil
	}
}
