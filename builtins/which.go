package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	afs "github.com/agentfs/afs"
	"github.com/agentfs/afs/mounts"
)

func builtinWhich(v *afs.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("which — show full path of command\nUsage: which <command>...\n")), nil
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("missing argument")
		}

		pathStr := afs.Env(ctx, "PATH")
		if pathStr == "" {
			pathStr = "/bin"
		}

		var output strings.Builder
		for _, cmd := range args {
			found := false
			dirs := strings.Split(pathStr, ":")
			for _, dir := range dirs {
				if dir == "" {
					continue
				}
				candidate := dir + "/" + cmd
				if entry, err := v.Stat(ctx, candidate); err == nil && entry.Perm.CanExec() {
					output.WriteString(candidate + "\n")
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("command not found: %s", cmd)
			}
		}
		return io.NopCloser(strings.NewReader(output.String())), nil
	}
}
