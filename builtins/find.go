package builtins

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

func builtinFind(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`find — search for files in a directory hierarchy
Usage: find [path...] [expression]

Expressions:
  -name PATTERN   File name matches glob pattern
  -type c         File type: f (regular file), d (directory)
  -maxdepth N     Descend at most N levels
  -mindepth N     Descend at least N levels
`)), nil
		}

		cwd := grasp.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		opts := findOptions{maxDepth: -1}
		searchPath := cwd
		var remainingArgs []string

		for i := 0; i < len(args); i++ {
			arg := args[i]
			if strings.HasPrefix(arg, "-") && arg != "-" && arg != "--" {
				switch arg {
				case "-name":
					if i+1 < len(args) {
						i++
						opts.name = args[i]
					}
				case "-type":
					if i+1 < len(args) {
						i++
						opts.fileType = args[i]
					}
				case "-path":
					if i+1 < len(args) {
						i++
						opts.path = args[i]
					}
				case "-maxdepth":
					if i+1 < len(args) {
						i++
						fmt.Sscanf(args[i], "%d", &opts.maxDepth)
					}
				case "-mindepth":
					if i+1 < len(args) {
						i++
						fmt.Sscanf(args[i], "%d", &opts.minDepth)
					}
				}
			} else if !strings.HasPrefix(arg, "-") {
				searchPath = resolvePath(cwd, arg)
				remainingArgs = args[i+1:]
				break
			}
		}

		for i := 0; i < len(remainingArgs); i++ {
			arg := remainingArgs[i]
			switch arg {
			case "-name":
				if i+1 < len(remainingArgs) {
					i++
					opts.name = remainingArgs[i]
				}
			case "-type":
				if i+1 < len(remainingArgs) {
					i++
					opts.fileType = remainingArgs[i]
				}
			case "-path":
				if i+1 < len(remainingArgs) {
					i++
					opts.path = remainingArgs[i]
				}
			case "-maxdepth":
				if i+1 < len(remainingArgs) {
					i++
					fmt.Sscanf(remainingArgs[i], "%d", &opts.maxDepth)
				}
			case "-mindepth":
				if i+1 < len(remainingArgs) {
					i++
					fmt.Sscanf(remainingArgs[i], "%d", &opts.minDepth)
				}
			}
		}

		var results []string
		err := findRecursive(ctx, v, searchPath, 0, opts, &results)
		if err != nil {
			return nil, fmt.Errorf("find: %w", err)
		}

		if len(results) == 0 {
			return io.NopCloser(strings.NewReader("")), nil
		}
		return io.NopCloser(strings.NewReader(strings.Join(results, "\n") + "\n")), nil
	}
}

type findOptions struct {
	name     string
	fileType string
	path     string
	maxDepth int
	minDepth int
}

func findRecursive(ctx context.Context, v *grasp.VirtualOS, dir string, depth int, opts findOptions, results *[]string) error {
	if opts.maxDepth >= 0 && depth > opts.maxDepth {
		return nil
	}
	if depth >= opts.minDepth {
		entry, err := v.Stat(ctx, dir)
		if err != nil {
			return nil
		}
		if matchesFindCriteria(entry, opts) {
			*results = append(*results, dir)
		}
	}

	if entry, err := v.Stat(ctx, dir); err == nil && entry.IsDir {
		entries, err := v.List(ctx, dir, grasp.ListOpts{})
		if err != nil {
			return nil
		}
		for _, e := range entries {
			childPath := dir
			if !strings.HasSuffix(dir, "/") {
				childPath += "/"
			}
			childPath += e.Name
			if err := findRecursive(ctx, v, childPath, depth+1, opts, results); err != nil {
				return err
			}
		}
	}
	return nil
}

func matchesFindCriteria(entry *grasp.Entry, opts findOptions) bool {
	if opts.fileType != "" {
		switch opts.fileType {
		case "f":
			if entry.IsDir {
				return false
			}
		case "d":
			if !entry.IsDir {
				return false
			}
		}
	}
	if opts.name != "" {
		matched, err := filepath.Match(opts.name, entry.Name)
		if err != nil || !matched {
			return false
		}
	}
	if opts.path != "" {
		matched, err := filepath.Match(opts.path, entry.Path)
		if err != nil || !matched {
			return false
		}
	}
	return true
}
