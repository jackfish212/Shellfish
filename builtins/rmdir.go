package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
	"github.com/jackfish212/shellfish/mounts"
)

func builtinRmdir(v *shellfish.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`rmdir — remove empty directories

Usage: rmdir [OPTION]... DIRECTORY...

Remove the DIRECTORY(ies), if they are empty.

Options:
  -p, --parents   Remove DIRECTORY and its ancestors; e.g., 'rmdir -p a/b/c' is
                  similar to 'rmdir a/b/c a/b a'
  --ignore-fail-on-non-empty
                  Ignore each failure that is solely because a directory is non-empty
  -v, --verbose   Output a diagnostic for every directory processed
`)), nil
		}

		parents := hasFlag(args, "-p", "--parents")
		ignoreNonEmpty := hasFlag(args, "--ignore-fail-on-non-empty")
		verbose := hasFlag(args, "-v", "--verbose")

		var paths []string
		for _, arg := range args {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			paths = append(paths, arg)
		}

		if len(paths) == 0 {
			return nil, fmt.Errorf("rmdir: missing operand")
		}

		cwd := shellfish.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		var out strings.Builder
		hasError := false

		for _, p := range paths {
			target := resolvePath(cwd, p)

			if parents {
				// Remove directory and all its empty parent directories
				hasErr := removeParents(ctx, v, target, ignoreNonEmpty, verbose, &out)
				if hasErr {
					hasError = true
				}
			} else {
				// Remove single directory
				if err := removeEmptyDir(ctx, v, target, &out, verbose); err != nil {
					if !ignoreNonEmpty || !isDirNotEmptyError(err) {
						fmt.Fprintf(&out, "rmdir: failed to remove '%s': %v\n", p, err)
						hasError = true
					}
				}
			}
		}

		if hasError {
			return io.NopCloser(strings.NewReader(out.String())), fmt.Errorf("rmdir: some directories could not be removed")
		}
		return io.NopCloser(strings.NewReader(out.String())), nil
	}
}

// removeEmptyDir removes a single empty directory
func removeEmptyDir(ctx context.Context, v *shellfish.VirtualOS, target string, out *strings.Builder, verbose bool) error {
	// Check if it's a directory
	entry, err := v.Stat(ctx, target)
	if err != nil {
		return fmt.Errorf("no such file or directory")
	}

	if !entry.IsDir {
		return fmt.Errorf("not a directory")
	}

	// Check if directory is empty
	entries, err := v.List(ctx, target, shellfish.ListOpts{})
	if err != nil {
		return err
	}

	if len(entries) > 0 {
		return fmt.Errorf("directory not empty")
	}

	// Remove the directory
	if err := v.Remove(ctx, target); err != nil {
		return err
	}

	if verbose {
		fmt.Fprintf(out, "rmdir: removing directory, '%s'\n", target)
	}

	return nil
}

// removeParents removes the directory and its empty parent directories
func removeParents(ctx context.Context, v *shellfish.VirtualOS, target string, ignoreNonEmpty bool, verbose bool, out *strings.Builder) bool {
	hasError := false
	current := target

	for {
		if current == "/" || current == "" {
			break
		}

		err := removeEmptyDir(ctx, v, current, out, verbose)
		if err != nil {
			if !ignoreNonEmpty || !isDirNotEmptyError(err) {
				// Get display name (relative if possible)
				displayName := strings.TrimPrefix(current, "/")
				if displayName == "" {
					displayName = current
				}
				fmt.Fprintf(out, "rmdir: failed to remove '%s': %v\n", displayName, err)
				hasError = true
			}
			break
		}

		// Move to parent directory
		parent := shellfish.CleanPath(current + "/..")
		if parent == current {
			break
		}
		current = parent
	}

	return hasError
}

// isDirNotEmptyError checks if the error is due to directory not being empty
func isDirNotEmptyError(err error) bool {
	return strings.Contains(err.Error(), "directory not empty") ||
		strings.Contains(err.Error(), "not empty")
}
