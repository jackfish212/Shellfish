package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
	"github.com/jackfish212/grasp/types"
)

func builtinMount(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(mountHelp())), nil
		}

		// If no arguments, list mount points
		if len(args) == 0 {
			return listMounts(v), nil
		}

		// Parse mount command: mount -t <type> [options] <source> <target>
		return performMount(ctx, v, args)
	}
}

func mountHelp() string {
	var buf strings.Builder
	buf.WriteString(`mount — mount filesystems or list mount points

Usage:
  mount                                    List all mount points
  mount -t <type> [options] <source> <target>  Mount a filesystem

Filesystem types:
`)

	// List all registered filesystem types
	types := ListMountTypes()
	for _, info := range types {
		buf.WriteString(fmt.Sprintf("  %-12s %s\n", info.Name, info.Description))
		buf.WriteString(fmt.Sprintf("              Example: %s\n\n", info.Usage))
	}

	buf.WriteString(`Options:
  -t <type>   Filesystem type
  -o <opts>   Mount options (comma-separated)
  -h, --help  Show this help message
`)
	return buf.String()
}

func listMounts(v *grasp.VirtualOS) io.ReadCloser {
	infos := v.MountTable().AllInfo()
	if len(infos) == 0 {
		return io.NopCloser(strings.NewReader("(no mounts)\n"))
	}
	var buf strings.Builder
	buf.WriteString("MountID   Type        Permissions  Source\n")
	buf.WriteString("--------  ----------  -----------  ------\n")
	for _, info := range infos {
		typ, extra := getMountInfo(info.Provider)
		buf.WriteString(formatMountInfo(info.Path, typ, info.Permissions, extra))
	}
	return io.NopCloser(strings.NewReader(buf.String()))
}

func performMount(ctx context.Context, v *grasp.VirtualOS, args []string) (io.ReadCloser, error) {
	var fsType, source, target string
	var options string

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-t":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("mount: -t requires an argument")
			}
			fsType = args[i+1]
			i++
		case "-o":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("mount: -o requires an argument")
			}
			options = args[i+1]
			i++
		default:
			if source == "" {
				source = args[i]
			} else if target == "" {
				target = args[i]
			} else {
				return nil, fmt.Errorf("mount: too many arguments")
			}
		}
	}

	if fsType == "" {
		return nil, fmt.Errorf("mount: filesystem type required (-t)")
	}
	if target == "" {
		return nil, fmt.Errorf("mount: target path required")
	}

	// Look up the mount handler from registry
	mountInfo, ok := GetMountType(fsType)
	if !ok {
		return nil, fmt.Errorf("mount: unknown filesystem type: %s", fsType)
	}

	// Parse options
	opts := parseOptions(options)

	// Call the registered handler
	if err := mountInfo.Handler(ctx, v, source, target, opts); err != nil {
		return nil, fmt.Errorf("mount: %w", err)
	}

	msg := fmt.Sprintf("Mounted %s at %s\n", fsType, target)
	return io.NopCloser(strings.NewReader(msg)), nil
}

func parseOptions(optStr string) map[string]string {
	opts := make(map[string]string)
	if optStr == "" {
		return opts
	}
	for _, opt := range strings.Split(optStr, ",") {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			continue
		}
		parts := strings.SplitN(opt, "=", 2)
		if len(parts) == 2 {
			opts[parts[0]] = parts[1]
		} else {
			opts[parts[0]] = "true"
		}
	}
	return opts
}

func getMountInfo(p grasp.Provider) (typ, extra string) {
	if mip, ok := p.(grasp.MountInfoProvider); ok {
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

// parsePermissions extracts permission from mount options
func parsePermissions(opts map[string]string) types.Perm {
	if opts["ro"] == "true" || opts["perm"] == "ro" {
		return types.PermRO
	}
	return types.PermRW
}
