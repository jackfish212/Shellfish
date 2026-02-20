package builtins

import (
	"context"
	"io"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

func builtinMount(v *shellfish.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader("mount — list mount points\nUsage: mount\n")), nil
		}
		infos := v.MountTable().AllInfo()
		if len(infos) == 0 {
			return io.NopCloser(strings.NewReader("(no mounts)\n")), nil
		}
		var buf strings.Builder
		buf.WriteString("MountID   Type        Permissions  Source\n")
		buf.WriteString("--------  ----------  -----------  ------\n")
		for _, info := range infos {
			typ, extra := getMountInfo(info.Provider)
			buf.WriteString(formatMountInfo(info.Path, typ, info.Permissions, extra))
		}
		return io.NopCloser(strings.NewReader(buf.String())), nil
	}
}

func getMountInfo(p shellfish.Provider) (typ, extra string) {
	if mip, ok := p.(shellfish.MountInfoProvider); ok {
		return mip.MountInfo()
	}
	return "unknown", "-"
}

func formatMountInfo(path, typ, perm, extra string) string {
	if extra == "" {
		extra = "-"
	}
	id := truncate(path, 8)
	t := truncate(typ, 10)
	return strings.Join([]string{pad(id, 8), pad(t, 10), pad(perm, 11), extra}, "  ") + "\n"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
