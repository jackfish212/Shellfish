package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

func builtinStat(v *shellfish.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("stat — show entry metadata\nUsage: stat <path>\n")), nil
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("stat: missing path")
		}
		cwd := shellfish.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}
		target := resolvePath(cwd, args[0])
		entry, err := v.Stat(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("stat: %w", err)
		}
		var buf strings.Builder
		fmt.Fprintf(&buf, "  Name: %s\n", entry.Name)
		fmt.Fprintf(&buf, "  Path: %s\n", entry.Path)
		fmt.Fprintf(&buf, "  Dir:  %v\n", entry.IsDir)
		fmt.Fprintf(&buf, "  Perm: %s\n", entry.Perm)
		if entry.Size > 0 {
			fmt.Fprintf(&buf, "  Size: %d\n", entry.Size)
		}
		if entry.MimeType != "" {
			fmt.Fprintf(&buf, "  Type: %s\n", entry.MimeType)
		}
		if !entry.Modified.IsZero() {
			fmt.Fprintf(&buf, "  Mod:  %s\n", entry.Modified.Format("2006-01-02 15:04:05"))
		}
		for k, val := range entry.Meta {
			fmt.Fprintf(&buf, "  %s: %s\n", k, val)
		}
		return io.NopCloser(strings.NewReader(buf.String())), nil
	}
}
