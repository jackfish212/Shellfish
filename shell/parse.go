package shell

import "strings"

func splitPipe(s string) []string {
	var segments []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case ch == '|' && !inSingle && !inDouble:
			segments = append(segments, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		segments = append(segments, current.String())
	}
	return segments
}

type logicalOp int

const (
	opNone logicalOp = iota
	opAnd
	opOr
)

type logicalSegment struct {
	cmd string
	op  logicalOp
}

func splitLogicalOps(s string) []logicalSegment {
	var segments []logicalSegment
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case ch == '&' && !inSingle && !inDouble:
			if i+1 < len(s) && s[i+1] == '&' {
				if current.Len() > 0 {
					segments = append(segments, logicalSegment{cmd: current.String(), op: opAnd})
					current.Reset()
				}
				i++
				continue
			}
			current.WriteByte(ch)
		case ch == '|' && !inSingle && !inDouble:
			if i+1 < len(s) && s[i+1] == '|' {
				if current.Len() > 0 {
					segments = append(segments, logicalSegment{cmd: current.String(), op: opOr})
					current.Reset()
				}
				i++
				continue
			}
			current.WriteByte(ch)
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		segments = append(segments, logicalSegment{cmd: current.String(), op: opNone})
	}

	if len(segments) == 0 {
		return nil
	}
	return segments
}

func splitBySemicolon(s string) []string {
	var commands []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case ch == ';' && !inSingle && !inDouble:
			if current.Len() > 0 {
				commands = append(commands, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		commands = append(commands, current.String())
	}
	return commands
}

type redirection struct {
	path           string
	append         bool
	isStderr       bool
	isCombined     bool
	stderrToStdout bool
}

func filterRedirectionArgs(args []string) []string {
	var result []string
	for i := 0; i < len(args); i++ {
		if args[i] == ">" || args[i] == ">>" || args[i] == "2>" || args[i] == "2>>" || args[i] == "&>" || args[i] == "&>>" {
			i++
			continue
		}
		if args[i] == "2>&1" {
			continue
		}
		result = append(result, args[i])
	}
	return result
}

func parseRedirection(s string) (*redirection, string) {
	var redir redirection
	var inSingle, inDouble bool
	var operatorPos int

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == '>' && !inSingle && !inDouble:
			operatorPos = i
			if i+1 < len(s) && s[i+1] == '&' {
				if i+2 < len(s) && s[i+2] == '>' {
					redir.append = true
					redir.isCombined = true
					i += 2
					goto parsePath
				}
			}
			if i+1 < len(s) && s[i+1] == '&' {
				redir.isCombined = true
				i++
				goto parsePath
			}
			if i+1 < len(s) && s[i+1] == '>' {
				redir.append = true
				i++
				goto parsePath
			}
			goto parsePath
		case ch == '2' && !inSingle && !inDouble:
			if i+1 < len(s) && s[i+1] == '>' {
				operatorPos = i
				if i+2 < len(s) && s[i+2] == '>' {
					redir.append = true
					redir.isStderr = true
					i += 2
					goto parsePath
				}
				redir.isStderr = true
				i++
				goto parsePath
			}
		}
		continue

	parsePath:
		i++
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			return nil, s[:operatorPos]
		}
		start := i
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
		redir.path = s[start:i]
		cmdPart := strings.TrimSpace(s[:operatorPos])
		return &redir, cmdPart
	}
	return nil, s
}

func parseStderrToStdout(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "2>&1") {
		cmdPart := strings.TrimSpace(s[:len(s)-4])
		return cmdPart, true
	}
	return s, false
}

func tokenize(s string) []string {
	tokens, _ := tokenizeWithQuoteInfo(s)
	return tokens
}

// tokenizeWithQuoteInfo splits a command line into tokens and tracks whether
// each token was (partially) quoted. Quoted tokens should not undergo glob expansion.
func tokenizeWithQuoteInfo(s string) ([]string, []bool) {
	var tokens []string
	var quoted []bool
	var current strings.Builder
	inSingle := false
	inDouble := false
	wasQuoted := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			wasQuoted = true
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			wasQuoted = true
		case (ch == ' ' || ch == '\t') && !inSingle && !inDouble:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				quoted = append(quoted, wasQuoted)
				current.Reset()
				wasQuoted = false
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
		quoted = append(quoted, wasQuoted)
	}
	return tokens, quoted
}

func filterRedirectionArgsWithQuotes(args []string, quoted []bool) ([]string, []bool) {
	var resultArgs []string
	var resultQuoted []bool
	for i := 0; i < len(args); i++ {
		if args[i] == ">" || args[i] == ">>" || args[i] == "2>" || args[i] == "2>>" || args[i] == "&>" || args[i] == "&>>" {
			i++
			continue
		}
		if args[i] == "2>&1" {
			continue
		}
		resultArgs = append(resultArgs, args[i])
		if i < len(quoted) {
			resultQuoted = append(resultQuoted, quoted[i])
		} else {
			resultQuoted = append(resultQuoted, false)
		}
	}
	return resultArgs, resultQuoted
}
