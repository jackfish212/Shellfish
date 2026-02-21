package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
)

// date — display the current date and time
func builtinDate(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`date — display the current date and time
Usage: date [+FORMAT]
Format specifiers:
  %Y - year (2024)
  %m - month (01-12)
  %d - day (01-31)
  %H - hour (00-23)
  %M - minute (00-59)
  %S - second (00-59)
  %s - Unix timestamp
  %T - time as HH:MM:SS
  %F - date as YYYY-MM-DD
`)), nil
		}

		now := time.Now()

		// Check for format argument
		for _, arg := range args {
			if strings.HasPrefix(arg, "+") {
				format := arg[1:]
				result := formatTimeString(now, format)
				return io.NopCloser(strings.NewReader(result + "\n")), nil
			}
		}

		// Default output
		return io.NopCloser(strings.NewReader(now.Format(time.RFC1123) + "\n")), nil
	}
}

func formatTimeString(t time.Time, format string) string {
	result := format
	result = strings.ReplaceAll(result, "%Y", t.Format("2006"))
	result = strings.ReplaceAll(result, "%y", t.Format("06"))
	result = strings.ReplaceAll(result, "%m", t.Format("01"))
	result = strings.ReplaceAll(result, "%d", t.Format("02"))
	result = strings.ReplaceAll(result, "%H", t.Format("15"))
	result = strings.ReplaceAll(result, "%M", t.Format("04"))
	result = strings.ReplaceAll(result, "%S", t.Format("05"))
	result = strings.ReplaceAll(result, "%s", fmt.Sprintf("%d", t.Unix()))
	result = strings.ReplaceAll(result, "%T", t.Format("15:04:05"))
	result = strings.ReplaceAll(result, "%F", t.Format("2006-01-02"))
	result = strings.ReplaceAll(result, "%R", t.Format("15:04"))
	result = strings.ReplaceAll(result, "%Z", t.Format("MST"))
	return result
}

// whoami — display the current user
func builtinWhoami(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`whoami — display the current user
Usage: whoami
`)), nil
		}

		user := grasp.Env(ctx, "USER")
		if user == "" {
			user = "unknown"
		}
		return io.NopCloser(strings.NewReader(user + "\n")), nil
	}
}

// sleep — delay for a specified amount of time
func builtinSleep(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`sleep — delay for a specified amount of time
Usage: sleep NUMBER[SUFFIX]
Suffix:
  s - seconds (default)
  m - minutes
  h - hours
`)), nil
		}

		if len(args) == 0 {
			return nil, fmt.Errorf("sleep: missing operand")
		}

		duration, err := parseDuration(args[0])
		if err != nil {
			return nil, fmt.Errorf("sleep: %w", err)
		}

		select {
		case <-time.After(duration):
			return io.NopCloser(strings.NewReader("")), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func parseDuration(s string) (time.Duration, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	var mult time.Duration = time.Second
	lastChar := s[len(s)-1]
	switch lastChar {
	case 's':
		s = s[:len(s)-1]
	case 'm':
		s = s[:len(s)-1]
		mult = time.Minute
	case 'h':
		s = s[:len(s)-1]
		mult = time.Hour
	}

	var num float64
	_, err := fmt.Sscanf(s, "%f", &num)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	return time.Duration(num * float64(mult)), nil
}

// true — return success exit status
func builtinTrue(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("")), nil
	}
}

// false — return failure exit status
func builtinFalse(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		return nil, fmt.Errorf("false")
	}
}

// whereis — locate the binary, source, and manual page files for a command
func builtinWhereis(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		if hasFlag(args, "-h", "--help") {
			return io.NopCloser(strings.NewReader(`whereis — locate command files
Usage: whereis COMMAND...
`)), nil
		}

		if len(args) == 0 {
			return io.NopCloser(strings.NewReader("")), nil
		}

		var result strings.Builder
		for _, cmd := range args {
			if strings.HasPrefix(cmd, "-") {
				continue
			}

			path := findCommand(v, cmd, ctx)
			if path != "" {
				result.WriteString(fmt.Sprintf("%s: %s\n", cmd, path))
			} else {
				result.WriteString(fmt.Sprintf("%s:\n", cmd))
			}
		}

		return io.NopCloser(strings.NewReader(result.String())), nil
	}
}

func findCommand(v *grasp.VirtualOS, cmd string, ctx context.Context) string {
	searchPaths := []string{"/bin", "/usr/bin", "/usr/local/bin"}

	for _, dir := range searchPaths {
		path := dir + "/" + cmd
		if _, err := v.Stat(ctx, path); err == nil {
			return path
		}
	}

	return ""
}
