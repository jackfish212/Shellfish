package builtins

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	grasp "github.com/jackfish212/grasp"
)

type grepOpts struct {
	ignoreCase bool
	invert     bool
	lineNumber bool
	count      bool
	recursive  bool
	wordMatch  bool
	context    int
	before     int
	after      int
	patterns   []string // -e patterns
}

type lineInfo struct {
	num     int
	text    string
	matched bool
}

func builtinGrep(v *grasp.VirtualOS) func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		opts := grepOpts{}
		pattern, files, err := parseGrepArgs(args, &opts)
		if err != nil {
			return nil, err
		}

		// Collect all patterns (from -e or positional arg)
		allPatterns := opts.patterns
		if pattern != "" {
			allPatterns = append(allPatterns, pattern)
		}

		if len(allPatterns) == 0 {
			return nil, fmt.Errorf("grep: missing pattern")
		}

		// Build regex - combine all patterns with alternation
		var regexPattern string
		if len(allPatterns) == 1 {
			regexPattern = allPatterns[0]
		} else {
			// Join patterns with alternation
			regexPattern = strings.Join(allPatterns, "|")
		}
		if opts.wordMatch {
			regexPattern = `\b(` + regexPattern + `)\b`
		}
		if opts.ignoreCase {
			regexPattern = "(?i)" + regexPattern
		}
		re, err := regexp.Compile(regexPattern)
		if err != nil {
			return nil, fmt.Errorf("grep: invalid pattern: %w", err)
		}

		// Get current working directory
		cwd := grasp.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		// Expand wildcards in file arguments
		files, err = expandWildcards(v, ctx, cwd, files)
		if err != nil {
			return nil, err
		}

		// Merge context options
		contextBefore := opts.before
		contextAfter := opts.after
		if opts.context > 0 {
			if contextBefore == 0 {
				contextBefore = opts.context
			}
			if contextAfter == 0 {
				contextAfter = opts.context
			}
		}

		var result strings.Builder

		// Read from stdin if no files specified
		if len(files) == 0 {
			if stdin == nil {
				return nil, fmt.Errorf("grep: no input")
			}
			matchCount := grepReaderWithCtx(stdin, re, &opts, "", &result, contextBefore, contextAfter)
			if opts.count {
				result.Reset()
				result.WriteString(fmt.Sprintf("%d\n", matchCount))
			}
			return io.NopCloser(strings.NewReader(result.String())), nil
		}

		// Process files
		totalCount := 0
		for _, file := range files {
			resolvedPath := resolvePath(cwd, file)

			count, err := grepPath(v, resolvedPath, file, re, &opts, &result, ctx, contextBefore, contextAfter)
			if err != nil {
				return nil, err
			}
			totalCount += count
		}

		if opts.count && len(files) == 1 {
			result.Reset()
			result.WriteString(fmt.Sprintf("%d\n", totalCount))
		}

		return io.NopCloser(strings.NewReader(result.String())), nil
	}
}

func parseGrepArgs(args []string, opts *grepOpts) (pattern string, files []string, err error) {
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-h", "--help":
			return "", nil, fmt.Errorf(`grep — search for patterns in files
Usage: grep [OPTIONS] PATTERN [FILE]...
Options:
  -i, --ignore-case   Ignore case distinctions
  -v, --invert-match  Select non-matching lines
  -n, --line-number   Print line number with output lines
  -c, --count         Print only a count of matching lines
  -r, -R, --recursive Recursively search directories
  -w, --word-regexp   Match only whole words
  -e, --regexp PATTERN  Specify pattern(s) to search (can be used multiple times)
  -C, --context NUM   Print NUM lines of context around matches
  -B, --before-context NUM Print NUM lines before matches
  -A, --after-context NUM  Print NUM lines after matches
`)
		case "-i", "--ignore-case":
			opts.ignoreCase = true
		case "-v", "--invert-match":
			opts.invert = true
		case "-n", "--line-number":
			opts.lineNumber = true
		case "-c", "--count":
			opts.count = true
		case "-r", "-R", "--recursive":
			opts.recursive = true
		case "-w", "--word-regexp":
			opts.wordMatch = true
		case "-e", "--regexp":
			if i+1 < len(args) {
				i++
				opts.patterns = append(opts.patterns, args[i])
			} else {
				return "", nil, fmt.Errorf("grep: option requires an argument: %s", args[i-1])
			}
		case "-C", "--context":
			if i+1 < len(args) {
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &opts.context); err != nil {
					return "", nil, fmt.Errorf("grep: invalid context argument: %s", args[i])
				}
			} else {
				return "", nil, fmt.Errorf("grep: option requires an argument: %s", args[i-1])
			}
		case "-B", "--before-context":
			if i+1 < len(args) {
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &opts.before); err != nil {
					return "", nil, fmt.Errorf("grep: invalid before-context argument: %s", args[i])
				}
			} else {
				return "", nil, fmt.Errorf("grep: option requires an argument: %s", args[i-1])
			}
		case "-A", "--after-context":
			if i+1 < len(args) {
				i++
				if _, err := fmt.Sscanf(args[i], "%d", &opts.after); err != nil {
					return "", nil, fmt.Errorf("grep: invalid after-context argument: %s", args[i])
				}
			} else {
				return "", nil, fmt.Errorf("grep: option requires an argument: %s", args[i-1])
			}
		default:
			if strings.HasPrefix(args[i], "-") && len(args[i]) > 1 && !isNumericArg(args[i]) {
				// Combined short flags like -in, or flags with numbers like -B1, -A2
				remaining := args[i][1:]
				for len(remaining) > 0 {
					c := rune(remaining[0])
					remaining = remaining[1:]
					switch c {
					case 'i':
						opts.ignoreCase = true
					case 'v':
						opts.invert = true
					case 'n':
						opts.lineNumber = true
					case 'c':
						opts.count = true
					case 'r', 'R':
						opts.recursive = true
					case 'w':
						opts.wordMatch = true
					case 'B':
						// Parse number that follows
						numStr := extractNumber(remaining)
						if numStr == "" {
							return "", nil, fmt.Errorf("grep: option requires a number: -B")
						}
						if _, err := fmt.Sscanf(numStr, "%d", &opts.before); err != nil {
							return "", nil, fmt.Errorf("grep: invalid number: %s", numStr)
						}
						remaining = remaining[len(numStr):]
					case 'A':
						// Parse number that follows
						numStr := extractNumber(remaining)
						if numStr == "" {
							return "", nil, fmt.Errorf("grep: option requires a number: -A")
						}
						if _, err := fmt.Sscanf(numStr, "%d", &opts.after); err != nil {
							return "", nil, fmt.Errorf("grep: invalid number: %s", numStr)
						}
						remaining = remaining[len(numStr):]
					case 'C':
						// Parse number that follows
						numStr := extractNumber(remaining)
						if numStr == "" {
							return "", nil, fmt.Errorf("grep: option requires a number: -C")
						}
						if _, err := fmt.Sscanf(numStr, "%d", &opts.context); err != nil {
							return "", nil, fmt.Errorf("grep: invalid number: %s", numStr)
						}
						remaining = remaining[len(numStr):]
					default:
						return "", nil, fmt.Errorf("grep: unknown option: -%c", c)
					}
				}
			} else {
				// First non-flag is pattern, rest are files
				if pattern == "" {
					pattern = args[i]
				} else {
					files = append(files, args[i])
				}
			}
		}
		i++
	}
	return pattern, files, nil
}

func isNumericArg(s string) bool {
	if len(s) < 2 {
		return false
	}
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// extractNumber extracts leading digits from a string
func extractNumber(s string) string {
	var result string
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result += string(c)
		} else {
			break
		}
	}
	return result
}

func grepReaderWithCtx(r io.Reader, re *regexp.Regexp, opts *grepOpts, filename string, result *strings.Builder, beforeCtx, afterCtx int) int {
	// Read all lines first for context support
	var lines []lineInfo
	scanner := bufio.NewScanner(r)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		text := scanner.Text()
		matched := re.MatchString(text)
		lines = append(lines, lineInfo{num: lineNum, text: text, matched: matched})
	}

	// If no context needed, use simple output
	if beforeCtx == 0 && afterCtx == 0 {
		matchCount := 0
		for _, l := range lines {
			if l.matched != opts.invert {
				matchCount++
				if !opts.count {
					writeLine(result, filename, l.num, l.text, opts)
				}
			}
		}
		return matchCount
	}

	// With context - find lines to print
	printLines := make(map[int]bool)
	matchCount := 0

	for i, l := range lines {
		if l.matched != opts.invert {
			matchCount++
			// Mark the matched line
			printLines[i] = true
			// Mark before context
			for j := 1; j <= beforeCtx && i-j >= 0; j++ {
				printLines[i-j] = true
			}
			// Mark after context
			for j := 1; j <= afterCtx && i+j < len(lines); j++ {
				printLines[i+j] = true
			}
		}
	}

	// Output with group separators
	lastPrinted := -2
	for i, l := range lines {
		if printLines[i] {
			if !opts.count {
				// Add separator for non-contiguous sections
				if lastPrinted >= 0 && i > lastPrinted+1 && (beforeCtx > 0 || afterCtx > 0) {
					result.WriteString("--\n")
				}
				writeLine(result, filename, l.num, l.text, opts)
			}
			lastPrinted = i
		}
	}

	return matchCount
}

func writeLine(result *strings.Builder, filename string, lineNum int, line string, opts *grepOpts) {
	if opts.lineNumber && filename != "" {
		result.WriteString(fmt.Sprintf("%s:%d:%s\n", filename, lineNum, line))
	} else if opts.lineNumber {
		result.WriteString(fmt.Sprintf("%d:%s\n", lineNum, line))
	} else if filename != "" {
		result.WriteString(fmt.Sprintf("%s:%s\n", filename, line))
	} else {
		result.WriteString(fmt.Sprintf("%s\n", line))
	}
}

func grepPath(v *grasp.VirtualOS, path, displayPath string, re *regexp.Regexp, opts *grepOpts, result *strings.Builder, ctx context.Context, beforeCtx, afterCtx int) (int, error) {
	entry, err := v.Stat(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("grep: %s: %w", displayPath, err)
	}

	if entry.IsDir {
		if !opts.recursive {
			return 0, fmt.Errorf("grep: %s: Is a directory", displayPath)
		}
		return grepDir(v, path, displayPath, re, opts, result, ctx, beforeCtx, afterCtx)
	}

	reader, err := v.Open(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("grep: %s: %w", displayPath, err)
	}
	defer func() { _ = reader.Close() }()

	count := grepReaderWithCtx(reader, re, opts, displayPath, result, beforeCtx, afterCtx)
	if opts.count {
		result.WriteString(fmt.Sprintf("%s:%d\n", displayPath, count))
	}
	return count, nil
}

func grepDir(v *grasp.VirtualOS, dirPath, displayPath string, re *regexp.Regexp, opts *grepOpts, result *strings.Builder, ctx context.Context, beforeCtx, afterCtx int) (int, error) {
	entries, err := v.List(ctx, dirPath, grasp.ListOpts{})
	if err != nil {
		return 0, fmt.Errorf("grep: %s: %w", displayPath, err)
	}

	totalCount := 0
	for _, entry := range entries {
		name := entry.Name
		childPath := dirPath + "/" + name
		childDisplay := displayPath + "/" + name

		count, err := grepPath(v, childPath, childDisplay, re, opts, result, ctx, beforeCtx, afterCtx)
		if err != nil {
			continue
		}
		totalCount += count
	}

	return totalCount, nil
}

// hasWildcard checks if a string contains wildcard characters
func hasWildcard(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// expandWildcards expands wildcard patterns in file arguments
func expandWildcards(v *grasp.VirtualOS, ctx context.Context, cwd string, files []string) ([]string, error) {
	if len(files) == 0 {
		return files, nil
	}

	var expanded []string
	for _, f := range files {
		// Resolve the file path relative to cwd
		dir := cwd
		pattern := f

		// If the path contains a directory component, separate them
		if idx := strings.LastIndex(f, "/"); idx >= 0 {
			dir = resolvePath(cwd, f[:idx])
			pattern = f[idx+1:]
		}

		// Check if pattern contains wildcards
		if !hasWildcard(pattern) {
			// No wildcards, use as-is
			expanded = append(expanded, f)
			continue
		}

		// List directory and match against pattern
		entries, err := v.List(ctx, dir, grasp.ListOpts{})
		if err != nil {
			// If we can't list the directory, keep the original path
			expanded = append(expanded, f)
			continue
		}

		matched := false
		for _, entry := range entries {
			m, err := filepath.Match(pattern, entry.Name)
			if err != nil {
				continue
			}
			if m {
				matched = true
				fullPath := dir
				if !strings.HasSuffix(fullPath, "/") {
					fullPath += "/"
				}
				fullPath += entry.Name
				// If original had directory prefix, preserve it in display
				if strings.HasPrefix(f, "/") || strings.Contains(f[:strings.Index(f, "/")], "/") {
					// Keep as absolute or relative path
				} else if idx := strings.LastIndex(f, "/"); idx >= 0 {
					fullPath = f[:idx+1] + entry.Name
				}
				expanded = append(expanded, fullPath)
			}
		}

		// If no matches found, keep the original pattern
		if !matched {
			expanded = append(expanded, f)
		}
	}

	return expanded, nil
}
