package builtins

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	shellfish "github.com/jackfish212/shellfish"
)

type grepOpts struct {
	ignoreCase bool
	invert     bool
	lineNumber bool
	count      bool
	recursive  bool
}

func builtinGrep(v *shellfish.VirtualOS) func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		opts := grepOpts{}
		pattern, files, err := parseGrepArgs(args, &opts)
		if err != nil {
			return nil, err
		}

		if pattern == "" {
			return nil, fmt.Errorf("grep: missing pattern")
		}

		// Build regex
		regexPattern := pattern
		if opts.ignoreCase {
			regexPattern = "(?i)" + regexPattern
		}
		re, err := regexp.Compile(regexPattern)
		if err != nil {
			return nil, fmt.Errorf("grep: invalid pattern: %w", err)
		}

		cwd := shellfish.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		var result strings.Builder

		// Read from stdin if no files specified
		if len(files) == 0 {
			if stdin == nil {
				return nil, fmt.Errorf("grep: no input")
			}
			matchCount := grepReader(stdin, re, &opts, "", &result)
			if opts.count {
				result.WriteString(fmt.Sprintf("%d\n", matchCount))
			}
			return io.NopCloser(strings.NewReader(result.String())), nil
		}

		// Process files
		totalCount := 0
		for _, file := range files {
			resolvedPath := resolvePath(cwd, file)

			count, err := grepPath(v, resolvedPath, file, re, &opts, &result, ctx)
			if err != nil {
				return nil, err
			}
			totalCount += count
		}

		if opts.count {
			if len(files) == 1 {
				result.WriteString(fmt.Sprintf("%d\n", totalCount))
			} else {
				// For multiple files, count was already written per file
			}
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
  -i, --ignore-case  Ignore case distinctions
  -v, --invert-match Select non-matching lines
  -n, --line-number  Print line number with output lines
  -c, --count        Print only a count of matching lines
  -r, -R, --recursive Recursively search directories
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
		default:
			if strings.HasPrefix(args[i], "-") && len(args[i]) > 1 {
				// Combined short flags like -in
				for _, c := range args[i][1:] {
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

func grepReader(r io.Reader, re *regexp.Regexp, opts *grepOpts, filename string, result *strings.Builder) int {
	scanner := bufio.NewScanner(r)
	lineNum := 0
	matchCount := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		matched := re.MatchString(line)

		if matched != opts.invert {
			matchCount++
			if !opts.count {
				writeLine(result, filename, lineNum, line, opts)
			}
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

func grepPath(v *shellfish.VirtualOS, path, displayPath string, re *regexp.Regexp, opts *grepOpts, result *strings.Builder, ctx context.Context) (int, error) {
	entry, err := v.Stat(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("grep: %s: %w", displayPath, err)
	}

	if entry.IsDir {
		if !opts.recursive {
			return 0, fmt.Errorf("grep: %s: Is a directory", displayPath)
		}
		return grepDir(v, path, displayPath, re, opts, result, ctx)
	}

	reader, err := v.Open(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("grep: %s: %w", displayPath, err)
	}
	defer reader.Close()

	count := grepReader(reader, re, opts, displayPath, result)
	if opts.count {
		result.WriteString(fmt.Sprintf("%s:%d\n", displayPath, count))
	}
	return count, nil
}

func grepDir(v *shellfish.VirtualOS, dirPath, displayPath string, re *regexp.Regexp, opts *grepOpts, result *strings.Builder, ctx context.Context) (int, error) {
	entries, err := v.List(ctx, dirPath, shellfish.ListOpts{})
	if err != nil {
		return 0, fmt.Errorf("grep: %s: %w", displayPath, err)
	}

	totalCount := 0
	for _, entry := range entries {
		name := entry.Name
		childPath := dirPath + "/" + name
		childDisplay := displayPath + "/" + name

		count, err := grepPath(v, childPath, childDisplay, re, opts, result, ctx)
		if err != nil {
			// Skip permission errors etc, continue with other files
			continue
		}
		totalCount += count
	}

	return totalCount, nil
}
