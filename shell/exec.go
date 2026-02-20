package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

func (s *Shell) execEnv() map[string]string {
	return map[string]string{
		"PWD":  s.Env.Get("PWD"),
		"PATH": s.Env.Get("PATH"),
		"USER": s.Env.Get("USER"),
		"HOME": s.Env.Get("HOME"),
	}
}

func (s *Shell) executeSingleStream(ctx context.Context, cmdLine string, stdin io.Reader) (io.ReadCloser, *ExecResult) {
	// Expand command substitutions first (`cmd` or $(cmd))
	cmdLine = s.expandCommandSubstitution(ctx, cmdLine)
	cmdLine = s.expandEnvVars(cmdLine)

	args, quoted := tokenizeWithQuoteInfo(cmdLine)
	for i := range args {
		args[i] = s.expandTilde(args[i])
	}
	if len(args) == 0 {
		return nil, &ExecResult{}
	}
	cmd := args[0]
	cmdArgs := s.expandGlobs(ctx, args[1:], quoted[1:])

	switch cmd {
	case "cd":
		result := s.cmdCd(cmdArgs)
		return io.NopCloser(strings.NewReader(result.Output)), nil
	case "pwd":
		return io.NopCloser(strings.NewReader(s.Env.Get("PWD") + "\n")), nil
	case "echo":
		result := s.cmdEcho(cmdArgs)
		return io.NopCloser(strings.NewReader(result.Output)), nil
	case "env":
		result := s.cmdEnv()
		return io.NopCloser(strings.NewReader(result.Output)), nil
	case "history":
		result := s.cmdHistory(cmdArgs)
		return io.NopCloser(strings.NewReader(result.Output)), nil
	}

	path, err := s.resolveCommand(ctx, cmd)
	if err != nil {
		return nil, &ExecResult{Output: err.Error() + "\n", Code: 1}
	}

	if entry, statErr := s.vos.Stat(ctx, path); statErr == nil && entry.IsDir {
		lsPath, lsErr := s.resolveCommand(ctx, "ls")
		if lsErr != nil {
			return nil, &ExecResult{Output: lsErr.Error() + "\n", Code: 1}
		}
		ctx = WithEnv(ctx, s.execEnv())
		rc, execErr := s.vos.Exec(ctx, lsPath, []string{path}, nil)
		if execErr != nil {
			return nil, &ExecResult{Output: fmt.Sprintf("ls: %v\n", execErr), Code: 1}
		}
		return rc, nil
	}

	ctx = WithEnv(ctx, s.execEnv())
	rc, execErr := s.vos.Exec(ctx, path, cmdArgs, stdin)
	if execErr != nil {
		return nil, &ExecResult{Output: fmt.Sprintf("%s: %v\n", cmd, execErr), Code: 1}
	}
	return rc, nil
}

func (s *Shell) executeSingle(ctx context.Context, cmdLine string, stdin io.Reader, redir *redirection) *ExecResult {
	slog.Debug("executeSingle called", "cmdLine", cmdLine, "hasRedir", redir != nil)
	// Expand command substitutions first (`cmd` or $(cmd))
	cmdLine = s.expandCommandSubstitution(ctx, cmdLine)
	cmdLine = s.expandEnvVars(cmdLine)

	args, quoted := tokenizeWithQuoteInfo(cmdLine)
	for i := range args {
		args[i] = s.expandTilde(args[i])
	}
	if len(args) == 0 {
		return &ExecResult{}
	}
	cmd := args[0]
	cmdArgs, cmdQuoted := filterRedirectionArgsWithQuotes(args[1:], quoted[1:])
	cmdArgs = s.expandGlobs(ctx, cmdArgs, cmdQuoted)

	switch cmd {
	case "cd":
		return s.cmdCd(cmdArgs)
	case "pwd":
		return &ExecResult{Output: s.Env.Get("PWD") + "\n"}
	case "echo":
		result := s.cmdEcho(cmdArgs)
		if redir != nil {
			return s.writeOutput(ctx, redir, result.Output)
		}
		return result
	case "env":
		return s.cmdEnv()
	case "history":
		return s.cmdHistory(cmdArgs)
	}

	path, err := s.resolveCommand(ctx, cmd)
	if err != nil {
		errMsg := err.Error() + "\n"
		if redir != nil {
			return s.writeOutput(ctx, redir, errMsg)
		}
		return &ExecResult{Output: errMsg, Code: 1}
	}

	if entry, statErr := s.vos.Stat(ctx, path); statErr == nil && entry.IsDir {
		lsPath, lsErr := s.resolveCommand(ctx, "ls")
		if lsErr != nil {
			return &ExecResult{Output: lsErr.Error() + "\n", Code: 1}
		}
		ctx = WithEnv(ctx, s.execEnv())
		rc, execErr := s.vos.Exec(ctx, lsPath, []string{path}, nil)
		if execErr != nil {
			return &ExecResult{Output: fmt.Sprintf("ls: %v\n", execErr), Code: 1}
		}
		defer rc.Close()
		var buf bytes.Buffer
		io.Copy(&buf, rc)
		output := buf.String()
		if redir != nil {
			return s.writeOutput(ctx, redir, output)
		}
		return &ExecResult{Output: output}
	}

	ctx = WithEnv(ctx, s.execEnv())
	rc, execErr := s.vos.Exec(ctx, path, cmdArgs, stdin)
	if execErr != nil {
		errMsg := fmt.Sprintf("%s: %v\n", cmd, execErr)
		if redir != nil {
			return s.writeOutput(ctx, redir, errMsg)
		}
		return &ExecResult{Output: errMsg, Code: 1}
	}
	defer rc.Close()
	var buf bytes.Buffer
	io.Copy(&buf, rc)
	output := buf.String()
	if redir != nil {
		return s.writeOutput(ctx, redir, output)
	}
	return &ExecResult{Output: output}
}

func (s *Shell) writeOutput(ctx context.Context, redir *redirection, output string) *ExecResult {
	targetPath := s.absPath(s.expandTilde(s.expandEnvVars(redir.path)))
	slog.Debug("writeOutput", "path", targetPath, "output", output)

	var r io.Reader
	if redir.append {
		if rc, err := s.vos.Open(ctx, targetPath); err == nil {
			defer rc.Close()
			existing, _ := io.ReadAll(rc)
			r = strings.NewReader(string(existing) + output)
		} else {
			r = strings.NewReader(output)
		}
	} else {
		r = strings.NewReader(output)
	}

	if err := s.vos.Write(ctx, targetPath, r); err != nil {
		return &ExecResult{Output: fmt.Sprintf("%s: %v\n", targetPath, err), Code: 1}
	}
	return &ExecResult{}
}

func (s *Shell) executeLogicalOps(ctx context.Context, segments []logicalSegment) *ExecResult {
	var output strings.Builder
	var lastCode int

	for _, seg := range segments {
		seg.cmd = strings.TrimSpace(seg.cmd)
		if seg.cmd == "" {
			continue
		}

		redir, cmdPart := parseRedirection(seg.cmd)
		cmdPart = strings.TrimSpace(cmdPart)
		if redir != nil {
			cmdPart, redir.stderrToStdout = parseStderrToStdout(cmdPart)
		}

		result := s.executeSingle(ctx, cmdPart, nil, redir)
		output.WriteString(result.Output)
		lastCode = result.Code

		switch seg.op {
		case opAnd:
			if result.Code != 0 {
				return &ExecResult{Output: output.String(), Code: result.Code}
			}
		case opOr:
			if result.Code == 0 {
				return &ExecResult{Output: output.String(), Code: 0}
			}
		case opNone:
			return &ExecResult{Output: output.String(), Code: result.Code}
		}
	}

	return &ExecResult{Output: output.String(), Code: lastCode}
}

func (s *Shell) executeCommandGroup(ctx context.Context, cmdLine string) *ExecResult {
	start := strings.Index(cmdLine, "{")
	end := strings.LastIndex(cmdLine, "}")
	if start == -1 || end == -1 || end <= start {
		return &ExecResult{Output: "invalid command group\n", Code: 1}
	}

	inner := cmdLine[start+1 : end]
	redir, _ := parseRedirection(cmdLine[end+1:])

	commands := splitBySemicolon(inner)

	var output strings.Builder
	var lastCode int

	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		result := s.Execute(ctx, cmd)
		output.WriteString(result.Output)
		lastCode = result.Code
	}

	if redir != nil && redir.path != "" {
		return s.writeOutput(ctx, redir, output.String())
	}

	return &ExecResult{Output: output.String(), Code: lastCode}
}
