package shell

import "strings"

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
