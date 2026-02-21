package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

const bindUsage = `bind — Plan 9-style union bind
Usage: bind [-b|-a] source_path target_path
  -b  bind source before target (source layer on top, e.g. cache)
  -a  bind source after target (source layer below, e.g. fallback)
  (no flag) replace target with source
`

func builtinBind(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(bindUsage)), nil
		}

		cwd := grasp.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		var before, after bool
		var filtered []string
		for _, a := range args {
			switch a {
			case "-b":
				before = true
			case "-a":
				after = true
			default:
				filtered = append(filtered, a)
			}
		}
		if len(filtered) != 2 {
			return nil, fmt.Errorf("bind: need exactly two paths (source and target), got %d", len(filtered))
		}
		sourcePath := resolvePath(cwd, filtered[0])
		targetPath := resolvePath(cwd, filtered[1])

		// Resolve both paths; they must be exact mount points (inner == "")
		_, innerTarget, errTarget := v.MountTable().Resolve(targetPath)
		if errTarget != nil {
			return nil, fmt.Errorf("bind: target %s: %w", targetPath, errTarget)
		}
		if innerTarget != "" {
			return nil, fmt.Errorf("bind: target %s is not a mount point", targetPath)
		}

		_, innerSource, errSource := v.MountTable().Resolve(sourcePath)
		if errSource != nil {
			return nil, fmt.Errorf("bind: source %s: %w", sourcePath, errSource)
		}
		if innerSource != "" {
			return nil, fmt.Errorf("bind: source %s is not a mount point", sourcePath)
		}

		currentProvider := providerAtPath(v, targetPath)
		newProvider := providerAtPath(v, sourcePath)
		if currentProvider == nil || newProvider == nil {
			return nil, fmt.Errorf("bind: could not get provider for %s or %s", sourcePath, targetPath)
		}

		if err := v.Unmount(targetPath); err != nil {
			return nil, fmt.Errorf("bind: unmount %s: %w", targetPath, err)
		}

		switch {
		case before:
			union := mounts.NewUnion(
				mounts.Layer{Provider: newProvider, Mode: mounts.BindBefore},
				mounts.Layer{Provider: currentProvider, Mode: mounts.BindAfter},
			)
			if err := v.Mount(targetPath, union); err != nil {
				return nil, fmt.Errorf("bind: mount %s: %w", targetPath, err)
			}
		case after:
			union := mounts.NewUnion(
				mounts.Layer{Provider: currentProvider, Mode: mounts.BindBefore},
				mounts.Layer{Provider: newProvider, Mode: mounts.BindAfter},
			)
			if err := v.Mount(targetPath, union); err != nil {
				return nil, fmt.Errorf("bind: mount %s: %w", targetPath, err)
			}
		default:
			if err := v.Mount(targetPath, newProvider); err != nil {
				return nil, fmt.Errorf("bind: mount %s: %w", targetPath, err)
			}
		}
		return io.NopCloser(strings.NewReader("")), nil
	}
}

// providerAtPath returns the Provider mounted exactly at path, or nil if not found.
func providerAtPath(v *grasp.VirtualOS, path string) grasp.Provider {
	path = grasp.CleanPath(path)
	for _, info := range v.MountTable().AllInfo() {
		if grasp.CleanPath(info.Path) == path {
			return info.Provider
		}
	}
	return nil
}
