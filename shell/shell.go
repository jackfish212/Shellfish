package shell

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
)

type hereDocInfo struct {
	delimiter string
	content   string
	quoted    bool
}

// Shell provides a command-line interface to AFS operations.
type Shell struct {
	vos         VirtualOS
	Env         *ShellEnv
	history     []string
	savedOffset int
}

// NewShell creates a Shell bound to a VirtualOS instance.
func NewShell(v VirtualOS, user string) *Shell {
	env := NewShellEnv()
	env.Set("USER", user)
	if user == "root" {
		env.Set("HOME", "/root")
	} else {
		env.Set("HOME", "/home/"+user)
	}
	env.Set("PWD", env.Get("HOME"))
	home := env.Get("HOME")
	env.Set("PATH", env.Get("PATH")+":"+home+"/.bin")
	sh := &Shell{vos: v, Env: env, history: []string{}}
	sh.loadProfileEnv()
	sh.loadHistory()
	return sh
}

// Cwd returns the current working directory.
func (s *Shell) Cwd() string {
	return s.Env.Get("PWD")
}

func (s *Shell) setCwd(path string) {
	s.Env.Set("PWD", path)
}

func (s *Shell) resolveCommand(ctx context.Context, cmd string) (string, error) {
	if strings.HasPrefix(cmd, "/") || strings.HasPrefix(cmd, "./") {
		return s.absPath(cmd), nil
	}
	pathStr := s.Env.Get("PATH")
	if pathStr == "" {
		pathStr = "/bin"
	}
	dirs := strings.Split(pathStr, ":")
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		var candidate string
		if dir == "/" {
			candidate = "/" + cmd
		} else {
			candidate = dir + "/" + cmd
		}
		if entry, err := s.vos.Stat(ctx, candidate); err == nil && entry.Perm.CanExec() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("command not found: %s", cmd)
}

// ExecResult holds the output of a shell command.
type ExecResult struct {
	Output string
	Code   int
}

func parseHereDoc(cmdLine string) (*hereDocInfo, string, string) {
	originalCmdLine := cmdLine
	lines := strings.SplitN(cmdLine, "\n", 2)
	firstLine := lines[0]

	var inSingle, inDouble bool
	hereDocIdx := -1

	for i := 0; i < len(firstLine); i++ {
		ch := firstLine[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == '<' && !inSingle && !inDouble:
			if i+1 < len(firstLine) && firstLine[i+1] == '<' {
				hereDocIdx = i
				break
			}
		}
	}

	if hereDocIdx == -1 {
		return nil, cmdLine, ""
	}

	cmdPart := strings.TrimSpace(firstLine[:hereDocIdx])
	i := hereDocIdx + 2
	for i < len(firstLine) && (firstLine[i] == ' ' || firstLine[i] == '\t') {
		i++
	}
	if i >= len(firstLine) {
		return nil, cmdLine, ""
	}

	var delim string
	quoted := false
	if firstLine[i] == '\'' || firstLine[i] == '"' {
		quoteChar := firstLine[i]
		quoted = true
		start := i + 1
		for j := start; j < len(firstLine); j++ {
			if firstLine[j] == quoteChar {
				delim = firstLine[start:j]
				i = j + 1
				break
			}
		}
	} else {
		start := i
		for i < len(firstLine) && firstLine[i] != ' ' && firstLine[i] != '\t' {
			i++
		}
		delim = firstLine[start:i]
	}

	for i < len(firstLine) && (firstLine[i] == ' ' || firstLine[i] == '\t') {
		i++
	}
	restOfFirstLine := ""
	if i < len(firstLine) {
		restOfFirstLine = strings.TrimSpace(firstLine[i:])
	}

	cmdWithRedir := cmdPart
	if restOfFirstLine != "" {
		cmdWithRedir = cmdPart + " " + restOfFirstLine
	}

	return &hereDocInfo{
		delimiter: delim,
		quoted:    quoted,
	}, cmdWithRedir, originalCmdLine
}

func extractHereDocContent(fullLine string, delim string) (content string, remaining string, err string) {
	lines := strings.Split(fullLine, "\n")
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == delim {
			content = strings.Join(lines[1:i], "\n")
			if i+1 < len(lines) {
				remaining = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			}
			return content, remaining, ""
		}
	}
	return "", "", "here-document delimiter '" + delim + "' not found"
}

// Execute parses and runs a command line.
func (s *Shell) Execute(ctx context.Context, cmdLine string) *ExecResult {
	cmdLine = strings.TrimSpace(cmdLine)
	if cmdLine == "" {
		return &ExecResult{}
	}

	s.addToHistory(cmdLine)

	if strings.HasPrefix(cmdLine, "{") && strings.Contains(cmdLine, "}") {
		return s.executeCommandGroup(ctx, cmdLine)
	}

	logicalSegs := splitLogicalOps(cmdLine)
	if len(logicalSegs) > 1 {
		return s.executeLogicalOps(ctx, logicalSegs)
	}

	var hereDoc *hereDocInfo
	var hereDocStdin io.Reader
	originalCmdLine := cmdLine

	hereDoc, cmdLine, _ = parseHereDoc(cmdLine)
	if hereDoc != nil {
		content, _, err := extractHereDocContent(originalCmdLine, hereDoc.delimiter)
		if err != "" {
			return &ExecResult{Output: err + "\n", Code: 1}
		}
		if !hereDoc.quoted {
			content = s.expandEnvVars(content)
		}
		hereDoc.content = content
		if content != "" && !strings.HasSuffix(content, "\n") {
			content = content + "\n"
		}
		hereDocStdin = strings.NewReader(content)
	}

	pipeSegs := splitPipe(cmdLine)

	if len(pipeSegs) == 1 {
		seg := strings.TrimSpace(pipeSegs[0])
		if seg == "" {
			return &ExecResult{}
		}
		redir, cmdPart := parseRedirection(seg)
		cmdPart = strings.TrimSpace(cmdPart)
		if redir != nil {
			cmdPart, redir.stderrToStdout = parseStderrToStdout(cmdPart)
		}
		stdin := hereDocStdin
		return s.executeSingle(ctx, cmdPart, stdin, redir)
	}

	var pipeInput io.Reader = hereDocStdin
	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()

	for i, seg := range pipeSegs {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		redir, cmdPart := parseRedirection(seg)
		cmdPart = strings.TrimSpace(cmdPart)
		if redir != nil {
			cmdPart, redir.stderrToStdout = parseStderrToStdout(cmdPart)
		}

		isLast := i == len(pipeSegs)-1
		if isLast {
			return s.executeSingle(ctx, cmdPart, pipeInput, redir)
		}

		rc, errResult := s.executeSingleStream(ctx, cmdPart, pipeInput)
		if errResult != nil {
			return errResult
		}
		closers = append(closers, rc)
		pipeInput = rc
	}

	return &ExecResult{}
}

func (s *Shell) absPath(p string) string {
	if strings.HasPrefix(p, "/") {
		return cleanPath(p)
	}
	p = strings.TrimPrefix(p, "./")
	if s.Env.Get("PWD") == "/" {
		return cleanPath("/" + p)
	}
	return cleanPath(s.Env.Get("PWD") + "/" + p)
}

// cleanPath normalises an AFS path: forward-slashes, no trailing slash,
// always starts with "/".
func cleanPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	if p == "." || p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}
