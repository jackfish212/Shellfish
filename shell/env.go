package shell

import (
	"context"
	"strings"
)

// ShellEnv provides environment variables for Shell.
type ShellEnv struct {
	data map[string]string
}

// NewShellEnv creates a new ShellEnv with default PATH, PWD, USER, and HOME.
func NewShellEnv() *ShellEnv {
	return &ShellEnv{data: map[string]string{
		"PATH": "/bin",
		"PWD":  "/",
		"USER": "root",
		"HOME": "/",
	}}
}

func (e *ShellEnv) Get(key string) string    { return e.data[key] }
func (e *ShellEnv) Set(key, value string)    { e.data[key] = value }

// All returns a copy of all environment variables.
func (e *ShellEnv) All() map[string]string {
	cp := make(map[string]string, len(e.data))
	for k, v := range e.data {
		cp[k] = v
	}
	return cp
}

func (s *Shell) expandEnvVars(cmdLine string) string {
	var result strings.Builder
	for i := 0; i < len(cmdLine); i++ {
		if cmdLine[i] == '$' && i+1 < len(cmdLine) {
			if i+1 < len(cmdLine) && cmdLine[i+1] == '{' {
				end := strings.Index(cmdLine[i+2:], "}")
				if end == -1 {
					result.WriteByte(cmdLine[i])
					i++
					continue
				}
				varName := cmdLine[i+2 : i+2+end]
				if val := s.Env.Get(varName); val != "" {
					result.WriteString(val)
				}
				i += 2 + end
				continue
			}
			start := i + 1
			for start < len(cmdLine) && isAlnumOrUnderscore(cmdLine[start]) {
				start++
			}
			if start == i+1 {
				result.WriteByte(cmdLine[i])
				i++
				continue
			}
			varName := cmdLine[i+1 : start]
			if val := s.Env.Get(varName); val != "" {
				result.WriteString(val)
			}
			i = start - 1
			continue
		}
		result.WriteByte(cmdLine[i])
	}
	return result.String()
}

func isAlnumOrUnderscore(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func (s *Shell) expandTilde(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	if len(path) == 1 || path[1] == '/' {
		home := s.Env.Get("HOME")
		if home == "" {
			return path
		}
		if len(path) == 1 {
			return home
		}
		return home + path[1:]
	}
	return path
}

// expandCommandSubstitution processes `cmd` style command substitution
func (s *Shell) expandCommandSubstitution(ctx context.Context, cmdLine string) string {
	var result strings.Builder
	inSingle := false
	i := 0

	for i < len(cmdLine) {
		ch := cmdLine[i]

		// Track single quotes - command substitution doesn't happen inside single quotes
		if ch == '\'' && !inSingle {
			inSingle = true
			result.WriteByte(ch)
			i++
			continue
		}
		if ch == '\'' && inSingle {
			inSingle = false
			result.WriteByte(ch)
			i++
			continue
		}

		// Backtick command substitution (not inside single quotes)
		if ch == '`' && !inSingle {
			// Find the closing backtick
			end := strings.Index(cmdLine[i+1:], "`")
			if end == -1 {
				// No closing backtick, keep as-is
				result.WriteByte(ch)
				i++
				continue
			}
			innerCmd := cmdLine[i+1 : i+1+end]
			// Execute the command and capture output
			output := s.executeCommandForSubstitution(ctx, innerCmd)
			result.WriteString(output)
			i += end + 2
			continue
		}

		// $(...) style command substitution (not inside single quotes)
		if ch == '$' && i+1 < len(cmdLine) && cmdLine[i+1] == '(' && !inSingle {
			// Find the matching closing paren
			depth := 1
			j := i + 2
			for j < len(cmdLine) && depth > 0 {
				if cmdLine[j] == '(' {
					depth++
				} else if cmdLine[j] == ')' {
					depth--
				}
				j++
			}
			if depth != 0 {
				// Unmatched parentheses, keep as-is
				result.WriteByte(ch)
				i++
				continue
			}
			innerCmd := cmdLine[i+2 : j-1]
			// Execute the command and capture output
			output := s.executeCommandForSubstitution(ctx, innerCmd)
			result.WriteString(output)
			i = j
			continue
		}

		result.WriteByte(ch)
		i++
	}

	return result.String()
}

// executeCommandForSubstitution runs a command and returns its output (trailing newlines stripped)
func (s *Shell) executeCommandForSubstitution(ctx context.Context, cmdLine string) string {
	result := s.Execute(ctx, cmdLine)
	// Strip trailing newlines (bash behavior for command substitution)
	output := strings.TrimRight(result.Output, "\n")
	// Replace remaining newlines with spaces (bash behavior)
	output = strings.ReplaceAll(output, "\n", " ")
	return output
}
