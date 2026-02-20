package shell

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func (s *Shell) cmdCd(args []string) *ExecResult {
	var target string
	if len(args) == 0 {
		target = s.Env.Get("HOME")
		if target == "" {
			target = "/"
		}
	} else {
		target = s.absPath(args[0])
	}
	ctx := context.Background()
	entry, err := s.vos.Stat(ctx, target)
	if err != nil {
		return &ExecResult{Output: fmt.Sprintf("cd: %s: No such file or directory\n", target), Code: 1}
	}
	if !entry.IsDir {
		return &ExecResult{Output: fmt.Sprintf("cd: %s: Not a directory\n", target), Code: 1}
	}
	s.Env.Set("PWD", target)
	return &ExecResult{}
}

func (s *Shell) cmdEcho(args []string) *ExecResult {
	return &ExecResult{Output: strings.Join(args, " ") + "\n"}
}

func (s *Shell) cmdEnv() *ExecResult {
	all := s.Env.All()
	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf strings.Builder
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(all[k])
		buf.WriteByte('\n')
	}
	return &ExecResult{Output: buf.String()}
}

func (s *Shell) cmdHistory(args []string) *ExecResult {
	if len(args) == 0 {
		var buf strings.Builder
		for i, entry := range s.history {
			cmd := ExtractCommand(entry)
			fmt.Fprintf(&buf, "%d %s\n", i+1, cmd)
		}
		return &ExecResult{Output: buf.String()}
	}

	switch args[0] {
	case "-c":
		s.history = nil
		return &ExecResult{}
	case "-d":
		if len(args) < 2 {
			return &ExecResult{Output: "history: -d requires an offset argument\n", Code: 1}
		}
		var offset int
		if _, err := fmt.Sscanf(args[1], "%d", &offset); err != nil {
			return &ExecResult{Output: "history: invalid offset\n", Code: 1}
		}
		idx := offset - 1
		if idx < 0 || idx >= len(s.history) {
			return &ExecResult{Output: "history: offset out of range\n", Code: 1}
		}
		s.history = append(s.history[:idx], s.history[idx+1:]...)
		return &ExecResult{}
	case "-a":
		return &ExecResult{}
	case "-n":
		s.history = nil
		s.loadHistory()
		return &ExecResult{}
	default:
		return &ExecResult{Output: "history: unknown option: " + args[0] + "\n", Code: 1}
	}
}
