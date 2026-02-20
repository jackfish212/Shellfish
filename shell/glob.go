package shell

import (
	"context"
	"path"
	"sort"
	"strings"

	"github.com/jackfish212/shellfish/types"
)

func hasGlobChars(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// expandGlobs expands wildcard patterns in command arguments.
// Quoted arguments are never expanded.
func (s *Shell) expandGlobs(ctx context.Context, args []string, quoted []bool) []string {
	var result []string
	for i, arg := range args {
		if i < len(quoted) && quoted[i] {
			result = append(result, arg)
			continue
		}
		if !hasGlobChars(arg) {
			result = append(result, arg)
			continue
		}
		matches := s.globMatch(ctx, arg)
		result = append(result, matches...)
	}
	return result
}

func (s *Shell) globMatch(ctx context.Context, pattern string) []string {
	isAbsolute := strings.HasPrefix(pattern, "/")

	var absPattern string
	if isAbsolute {
		absPattern = path.Clean(pattern)
	} else {
		cwd := s.Cwd()
		if cwd == "/" {
			absPattern = "/" + pattern
		} else {
			absPattern = cwd + "/" + pattern
		}
		absPattern = path.Clean(absPattern)
	}
	if !strings.HasPrefix(absPattern, "/") {
		absPattern = "/" + absPattern
	}

	parts := strings.Split(strings.TrimPrefix(absPattern, "/"), "/")
	matches := s.globRecurse(ctx, "/", parts)

	if len(matches) == 0 {
		return []string{pattern}
	}

	if !isAbsolute {
		cwd := s.Cwd()
		prefix := cwd
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		for i, m := range matches {
			if strings.HasPrefix(m, prefix) {
				matches[i] = m[len(prefix):]
			}
		}
	}

	sort.Strings(matches)
	return matches
}

func (s *Shell) globRecurse(ctx context.Context, base string, parts []string) []string {
	if len(parts) == 0 {
		return []string{base}
	}

	part := parts[0]
	rest := parts[1:]

	if !hasGlobChars(part) {
		var next string
		if base == "/" {
			next = "/" + part
		} else {
			next = base + "/" + part
		}
		if len(rest) == 0 {
			if _, err := s.vos.Stat(ctx, next); err != nil {
				return nil
			}
			return []string{next}
		}
		return s.globRecurse(ctx, next, rest)
	}

	entries, err := s.vos.List(ctx, base, types.ListOpts{})
	if err != nil {
		return nil
	}

	var results []string
	for _, entry := range entries {
		matched, merr := path.Match(part, entry.Name)
		if merr != nil || !matched {
			continue
		}
		var next string
		if base == "/" {
			next = "/" + entry.Name
		} else {
			next = base + "/" + entry.Name
		}
		if len(rest) == 0 {
			results = append(results, next)
		} else if entry.IsDir {
			sub := s.globRecurse(ctx, next, rest)
			results = append(results, sub...)
		}
	}

	return results
}
