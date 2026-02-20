package builtins

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	afs "github.com/agentfs/afs"
	"github.com/agentfs/afs/mounts"
)

func builtinTail(v *afs.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`tail — output the last part of files
Usage: tail [OPTION]... [FILE]...
Options:
  -n, --lines=NUMBER   Number of lines (default: 10)
  -c, --bytes=NUMBER   Number of bytes
`)), nil
		}

		cwd := afs.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		var lines int = 10
		var bytes int64 = -1
		var files []string

		for i := 0; i < len(args); i++ {
			arg := args[i]
			if arg == "-n" || arg == "--lines" {
				if i+1 < len(args) {
					i++
					n, err := strconv.Atoi(args[i])
					if err != nil {
						return nil, fmt.Errorf("tail: invalid number of lines: %s", args[i])
					}
					lines = n
				}
			} else if strings.HasPrefix(arg, "--lines=") {
				n, err := strconv.Atoi(strings.TrimPrefix(arg, "--lines="))
				if err != nil {
					return nil, fmt.Errorf("tail: invalid number of lines: %s", arg)
				}
				lines = n
			} else if arg == "-c" || arg == "--bytes" {
				if i+1 < len(args) {
					i++
					n, err := strconv.ParseInt(args[i], 10, 64)
					if err != nil {
						return nil, fmt.Errorf("tail: invalid number of bytes: %s", args[i])
					}
					bytes = n
				}
			} else if strings.HasPrefix(arg, "--bytes=") {
				n, err := strconv.ParseInt(strings.TrimPrefix(arg, "--bytes="), 10, 64)
				if err != nil {
					return nil, fmt.Errorf("tail: invalid number of bytes: %s", arg)
				}
				bytes = n
			} else if !strings.HasPrefix(arg, "-") {
				files = append(files, resolvePath(cwd, arg))
			}
		}

		if len(files) == 0 {
			if stdin == nil {
				return nil, fmt.Errorf("tail: missing file operand")
			}
			data, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("tail: read error: %w", err)
			}
			content := string(data)
			if bytes >= 0 {
				start := int64(len(content)) - bytes
				if start < 0 {
					start = 0
				}
				content = content[start:]
			} else {
				allLines := strings.Split(content, "\n")
				if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
					allLines = allLines[:len(allLines)-1]
				}
				start := len(allLines) - lines
				if start < 0 {
					start = 0
				}
				lastLines := allLines[start:]
				if len(lastLines) > 0 {
					content = strings.Join(lastLines, "\n") + "\n"
				}
			}
			return io.NopCloser(strings.NewReader(content)), nil
		}

		var results []string
		for idx, file := range files {
			rc, err := v.Open(ctx, file)
			if err != nil {
				return nil, fmt.Errorf("tail: %w", err)
			}
			defer rc.Close()

			var content string
			if bytes >= 0 {
				data, err := io.ReadAll(rc)
				if err != nil {
					return nil, fmt.Errorf("tail: read error: %w", err)
				}
				start := int64(len(data)) - bytes
				if start < 0 {
					start = 0
				}
				content = string(data[start:])
			} else {
				scanner := bufio.NewScanner(rc)
				var allLines []string
				for scanner.Scan() {
					allLines = append(allLines, scanner.Text())
				}
				start := len(allLines) - lines
				if start < 0 {
					start = 0
				}
				lastLines := allLines[start:]
				if len(lastLines) > 0 {
					content = strings.Join(lastLines, "\n") + "\n"
				}
			}

			if len(files) > 1 {
				results = append(results, fmt.Sprintf("==> %s <==", file))
			}
			results = append(results, content)
			if idx < len(files)-1 && len(files) > 1 {
				results = append(results, "")
			}
		}
		return io.NopCloser(strings.NewReader(strings.Join(results, ""))), nil
	}
}
