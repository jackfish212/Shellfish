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
	noNewline := false
	enableEscape := false
	i := 0

	// Parse options
	for i < len(args) {
		if args[i] == "-n" {
			noNewline = true
			i++
		} else if args[i] == "-e" {
			enableEscape = true
			i++
		} else if args[i] == "-E" {
			enableEscape = false
			i++
		} else if args[i] == "-ne" || args[i] == "-en" {
			noNewline = true
			enableEscape = true
			i++
		} else if args[i] == "-nE" || args[i] == "-En" {
			noNewline = true
			enableEscape = false
			i++
		} else if strings.HasPrefix(args[i], "-") {
			// Check for combined flags like -neE
			combined := args[i][1:]
			validFlags := true
			for _, ch := range combined {
				if ch != 'n' && ch != 'e' && ch != 'E' {
					validFlags = false
					break
				}
			}
			if validFlags {
				for _, ch := range combined {
					switch ch {
					case 'n':
						noNewline = true
					case 'e':
						enableEscape = true
					case 'E':
						enableEscape = false
					}
				}
				i++
			} else {
				break
			}
		} else {
			break
		}
	}

	output := strings.Join(args[i:], " ")

	if enableEscape {
		output = processEchoEscapes(output)
	}

	if !noNewline {
		output += "\n"
	}

	return &ExecResult{Output: output}
}

// processEchoEscapes handles escape sequences like \n, \t, \\, etc.
func processEchoEscapes(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				result.WriteByte('\n')
				i += 2
			case 't':
				result.WriteByte('\t')
				i += 2
			case 'r':
				result.WriteByte('\r')
				i += 2
			case '\\':
				result.WriteByte('\\')
				i += 2
			case 'a':
				result.WriteByte('\a') // Bell/alert
				i += 2
			case 'b':
				result.WriteByte('\b') // Backspace
				i += 2
			case 'f':
				result.WriteByte('\f') // Form feed
				i += 2
			case 'v':
				result.WriteByte('\v') // Vertical tab
				i += 2
			case '0':
				// Octal escape \0NNN
				if i+2 < len(s) && s[i+2] >= '0' && s[i+2] <= '7' {
					val := int(s[i+2] - '0')
					if i+3 < len(s) && s[i+3] >= '0' && s[i+3] <= '7' {
						val = val*8 + int(s[i+3]-'0')
						if i+4 < len(s) && s[i+4] >= '0' && s[i+4] <= '7' {
							val = val*8 + int(s[i+4]-'0')
							result.WriteByte(byte(val))
							i += 5
							continue
						}
						result.WriteByte(byte(val))
						i += 4
						continue
					}
					result.WriteByte(byte(val))
					i += 3
					continue
				}
				result.WriteByte(0)
				i += 2
			case 'x':
				// Hex escape \xNN
				if i+2 < len(s) {
					val := 0
					hexChars := 0
					for j := i + 2; j < len(s) && hexChars < 2; j++ {
						ch := s[j]
						if ch >= '0' && ch <= '9' {
							val = val*16 + int(ch-'0')
							hexChars++
						} else if ch >= 'a' && ch <= 'f' {
							val = val*16 + int(ch-'a'+10)
							hexChars++
						} else if ch >= 'A' && ch <= 'F' {
							val = val*16 + int(ch-'A'+10)
							hexChars++
						} else {
							break
						}
					}
					if hexChars > 0 {
						result.WriteByte(byte(val))
						i += 2 + hexChars
						continue
					}
				}
				result.WriteByte('\\')
				i++
			case 'c':
				// \c suppresses further output and newline (like bash)
				return result.String()
			default:
				// Unknown escape, keep as is
				result.WriteByte(s[i])
				i++
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
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
