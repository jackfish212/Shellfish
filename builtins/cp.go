package builtins

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

func builtinCp(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`cp — copy files
Usage: cp [-r] <source> <dest>
       cp [-r] <source>... <directory>

Options:
  -r    Copy directories recursively
`)), nil
		}

		// Parse flags
		recursive := false
		var paths []string
		for _, arg := range args {
			if arg == "-r" || arg == "-R" {
				recursive = true
				continue
			}
			if strings.HasPrefix(arg, "-") && arg != "-" {
				continue
			}
			paths = append(paths, arg)
		}

		if len(paths) < 2 {
			return nil, fmt.Errorf("cp: missing operand")
		}

		cwd := grasp.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		// Last argument is the destination
		dst := resolvePath(cwd, paths[len(paths)-1])
		srcs := paths[:len(paths)-1]

		// Check if destination is a directory
		dstEntry, dstErr := v.Stat(ctx, dst)
		dstIsDir := dstErr == nil && dstEntry.IsDir

		var out strings.Builder

		for _, src := range srcs {
			srcPath := resolvePath(cwd, src)
			if err := copyEntry(ctx, v, srcPath, dst, dstIsDir, recursive, &out); err != nil {
				return nil, err
			}
		}

		return io.NopCloser(strings.NewReader(out.String())), nil
	}
}

// copyEntry copies a file or directory from src to dst
func copyEntry(ctx context.Context, v *grasp.VirtualOS, src, dst string, dstIsDir, recursive bool, out *strings.Builder) error {
	srcEntry, err := v.Stat(ctx, src)
	if err != nil {
		return fmt.Errorf("cp: cannot stat %q: %w", src, err)
	}

	// Determine target path
	targetDst := dst
	if dstIsDir {
		targetDst = path.Join(dst, srcEntry.Name)
	}

	if srcEntry.IsDir {
		if !recursive {
			return fmt.Errorf("cp: -r not specified; omitting directory %q", src)
		}
		return copyDir(ctx, v, src, targetDst, out)
	}

	return copyFile(ctx, v, src, targetDst, out)
}

// copyFile copies a single file
func copyFile(ctx context.Context, v *grasp.VirtualOS, src, dst string, out *strings.Builder) error {
	// Open source file
	rc, err := v.Open(ctx, src)
	if err != nil {
		return fmt.Errorf("cp: cannot open %q: %w", src, err)
	}
	defer rc.Close()

	// Write to destination
	if err := v.Write(ctx, dst, rc); err != nil {
		return fmt.Errorf("cp: cannot write to %q: %w", dst, err)
	}

	fmt.Fprintf(out, "copied: %s -> %s\n", src, dst)
	return nil
}

// copyDir recursively copies a directory
func copyDir(ctx context.Context, v *grasp.VirtualOS, src, dst string, out *strings.Builder) error {
	// Create destination directory
	if err := v.Mkdir(ctx, dst, grasp.PermRWX); err != nil {
		return fmt.Errorf("cp: cannot create directory %q: %w", dst, err)
	}

	// List source directory contents
	entries, err := v.List(ctx, src, grasp.ListOpts{})
	if err != nil {
		return fmt.Errorf("cp: cannot list %q: %w", src, err)
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := path.Join(src, entry.Name)
		dstPath := path.Join(dst, entry.Name)

		if entry.IsDir {
			if err := copyDir(ctx, v, srcPath, dstPath, out); err != nil {
				return err
			}
		} else {
			if err := copyFile(ctx, v, srcPath, dstPath, out); err != nil {
				return err
			}
		}
	}

	fmt.Fprintf(out, "copied: %s -> %s\n", src, dst)
	return nil
}
