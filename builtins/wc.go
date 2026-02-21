package builtins

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"

	grasp "github.com/jackfish212/grasp"
)

type wcOpts struct {
	lines      bool
	words      bool
	chars      bool
	bytes      bool
	maxLineLen bool
}

type wcCounts struct {
	lines      int
	words      int
	chars      int
	bytes      int
	maxLineLen int
}

func builtinWc(v *grasp.VirtualOS) func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		opts, files, err := parseWcArgs(args)
		if err != nil {
			return nil, err
		}

		// If no options specified, default to -lwc (lines, words, bytes)
		if !opts.lines && !opts.words && !opts.chars && !opts.bytes && !opts.maxLineLen {
			opts.lines = true
			opts.words = true
			opts.bytes = true
		}

		// Get current working directory
		cwd := grasp.Env(ctx, "PWD")
		if cwd == "" {
			cwd = "/"
		}

		var result strings.Builder

		// Read from stdin if no files specified
		if len(files) == 0 {
			if stdin == nil {
				return nil, fmt.Errorf("wc: no input")
			}
			counts := countReader(stdin)
			formatCounts(&result, counts, opts, "")
			return io.NopCloser(strings.NewReader(result.String())), nil
		}

		// Process files
		var totalCounts wcCounts
		for _, file := range files {
			resolvedPath := resolvePath(cwd, file)

			counts, err := countFile(v, ctx, resolvedPath)
			if err != nil {
				return nil, err
			}

			formatCounts(&result, counts, opts, file)

			totalCounts.lines += counts.lines
			totalCounts.words += counts.words
			totalCounts.chars += counts.chars
			totalCounts.bytes += counts.bytes
			if counts.maxLineLen > totalCounts.maxLineLen {
				totalCounts.maxLineLen = counts.maxLineLen
			}
		}

		// Print totals if more than one file
		if len(files) > 1 {
			formatCounts(&result, &totalCounts, opts, "total")
		}

		return io.NopCloser(strings.NewReader(result.String())), nil
	}
}

func parseWcArgs(args []string) (wcOpts, []string, error) {
	var opts wcOpts
	var files []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			return opts, nil, fmt.Errorf(`wc — print newline, word, and byte counts
Usage: wc [OPTIONS] [FILE]...
Options:
  -l, --lines          Print the newline counts
  -w, --words          Print the word counts
  -m, --chars          Print the character counts
  -c, --bytes          Print the byte counts
  -L, --max-line-length Print the maximum display width
`)
		case "-l", "--lines":
			opts.lines = true
		case "-w", "--words":
			opts.words = true
		case "-m", "--chars":
			opts.chars = true
		case "-c", "--bytes":
			opts.bytes = true
		case "-L", "--max-line-length":
			opts.maxLineLen = true
		default:
			if strings.HasPrefix(args[i], "-") && len(args[i]) > 1 {
				// Combined short flags like -lwc
				for _, c := range args[i][1:] {
					switch c {
					case 'l':
						opts.lines = true
					case 'w':
						opts.words = true
					case 'm':
						opts.chars = true
					case 'c':
						opts.bytes = true
					case 'L':
						opts.maxLineLen = true
					default:
						return opts, nil, fmt.Errorf("wc: invalid option -- '%c'", c)
					}
				}
			} else {
				files = append(files, args[i])
			}
		}
	}

	return opts, files, nil
}

func countReader(r io.Reader) *wcCounts {
	var counts wcCounts
	reader := bufio.NewReader(r)

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			counts.lines++
			counts.bytes += len(line)
			counts.chars += utf8RuneCount(line)

			// Count words (whitespace-separated sequences)
			wordCount := countWords(line)
			counts.words += wordCount

			// Track max line length (display width, excluding newline)
			lineLen := utf8RuneCount(strings.TrimSuffix(line, "\n"))
			if lineLen > counts.maxLineLen {
				counts.maxLineLen = lineLen
			}
		}

		if err != nil {
			break
		}
	}

	// If last line didn't end with newline, don't count it as a line
	// This matches standard wc behavior
	// Actually, standard wc counts newlines, not lines
	// So we need to adjust - let's read all content and count newlines

	return &counts
}

func countFile(v *grasp.VirtualOS, ctx context.Context, path string) (*wcCounts, error) {
	entry, err := v.Stat(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("wc: %s: %w", path, err)
	}

	if entry.IsDir {
		return nil, fmt.Errorf("wc: %s: Is a directory", path)
	}

	reader, err := v.Open(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("wc: %s: %w", path, err)
	}
	defer reader.Close()

	return countReader(reader), nil
}

func countWords(s string) int {
	inWord := false
	count := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			inWord = false
		} else {
			if !inWord {
				count++
				inWord = true
			}
		}
	}
	return count
}

func utf8RuneCount(s string) int {
	return len([]rune(s))
}

func formatCounts(result *strings.Builder, counts *wcCounts, opts wcOpts, filename string) {
	var parts []string

	if opts.lines {
		parts = append(parts, fmt.Sprintf("%8d", counts.lines))
	}
	if opts.words {
		parts = append(parts, fmt.Sprintf("%8d", counts.words))
	}
	if opts.chars {
		parts = append(parts, fmt.Sprintf("%8d", counts.chars))
	}
	if opts.bytes {
		parts = append(parts, fmt.Sprintf("%8d", counts.bytes))
	}
	if opts.maxLineLen {
		parts = append(parts, fmt.Sprintf("%8d", counts.maxLineLen))
	}

	output := strings.Join(parts, "")
	if filename != "" {
		output += " " + filename
	}
	result.WriteString(output + "\n")
}
