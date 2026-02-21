package builtins

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

func builtinHead(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`head — output the first part of files
Usage: head [OPTION]... [FILE]...
Options:
  -n, --lines=NUMBER   Number of lines (default: 10)
  -c, --bytes=NUMBER   Number of bytes
`)), nil
		}

		cwd := grasp.Env(ctx, "PWD")
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
						return nil, fmt.Errorf("head: invalid number of lines: %s", args[i])
					}
					lines = n
				}
			} else if strings.HasPrefix(arg, "--lines=") {
				n, err := strconv.Atoi(strings.TrimPrefix(arg, "--lines="))
				if err != nil {
					return nil, fmt.Errorf("head: invalid number of lines: %s", arg)
				}
				lines = n
			} else if arg == "-c" || arg == "--bytes" {
				if i+1 < len(args) {
					i++
					n, err := strconv.ParseInt(args[i], 10, 64)
					if err != nil {
						return nil, fmt.Errorf("head: invalid number of bytes: %s", args[i])
					}
					bytes = n
				}
			} else if strings.HasPrefix(arg, "--bytes=") {
				n, err := strconv.ParseInt(strings.TrimPrefix(arg, "--bytes="), 10, 64)
				if err != nil {
					return nil, fmt.Errorf("head: invalid number of bytes: %s", arg)
				}
				bytes = n
			} else if !strings.HasPrefix(arg, "-") {
				files = append(files, resolvePath(cwd, arg))
			}
		}

		if len(files) == 0 {
			if stdin == nil {
				return nil, fmt.Errorf("head: missing file operand")
			}
			data, err := io.ReadAll(stdin)
			if err != nil {
				return nil, fmt.Errorf("head: read error: %w", err)
			}
			content := string(data)
			if bytes >= 0 {
				if int64(len(content)) > bytes {
					content = content[:bytes]
				}
			} else {
				allLines := strings.Split(content, "\n")
				if len(allLines) > lines {
					allLines = allLines[:lines]
				}
				content = strings.Join(allLines, "\n")
				if len(allLines) > 0 && !strings.HasSuffix(content, "\n") {
					content += "\n"
				}
			}
			return io.NopCloser(strings.NewReader(content)), nil
		}

		var results []string
		for idx, file := range files {
			rc, err := v.Open(ctx, file)
			if err != nil {
				return nil, fmt.Errorf("head: %w", err)
			}
			defer rc.Close()

			var content string
			if bytes >= 0 {
				buf := make([]byte, bytes)
				n, _ := io.ReadFull(rc, buf)
				if n > 0 {
					content = string(buf[:n])
				}
			} else {
				scanner := bufio.NewScanner(rc)
				var linesCollected []string
				for scanner.Scan() && len(linesCollected) < lines {
					linesCollected = append(linesCollected, scanner.Text())
				}
				content = strings.Join(linesCollected, "\n")
				if len(linesCollected) > 0 {
					content += "\n"
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
