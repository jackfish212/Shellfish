package builtins

import (
	"context"
	"fmt"
	"io"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/rwtodd/Go.Sed/sed"
)

type sedOpts struct {
	quiet   bool
	expr    string
	file    string
	inPlace bool
}

func builtinSed(v *grasp.VirtualOS) func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		opts := sedOpts{}
		files, err := parseSedArgs(args, &opts)
		if err != nil {
			return nil, err
		}

		// Build the sed program
		var program string
		if opts.file != "" {
			// Read sed program from file
			cwd := grasp.Env(ctx, "PWD")
			if cwd == "" {
				cwd = "/"
			}
			resolvedPath := resolvePath(cwd, opts.file)
			reader, err := v.Open(ctx, resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("sed: can't read %s: %w", opts.file, err)
			}
			defer reader.Close()
			content, err := io.ReadAll(reader)
			if err != nil {
				return nil, fmt.Errorf("sed: can't read %s: %w", opts.file, err)
			}
			program = string(content)
		} else if opts.expr != "" {
			program = opts.expr
		} else {
			return nil, fmt.Errorf("sed: no script specified")
		}

		// Create the sed engine
		var engine *sed.Engine
		var sedErr error
		if opts.quiet {
			engine, sedErr = sed.NewQuiet(strings.NewReader(program))
		} else {
			engine, sedErr = sed.New(strings.NewReader(program))
		}
		if sedErr != nil {
			return nil, fmt.Errorf("sed: %w", sedErr)
		}

		// Handle in-place editing
		if opts.inPlace {
			if len(files) == 0 {
				return nil, fmt.Errorf("sed: -i requires input files")
			}
			return sedInPlace(v, engine, files, ctx)
		}

		// Process stdin or files
		var result strings.Builder

		if len(files) == 0 {
			if stdin == nil {
				return nil, fmt.Errorf("sed: no input")
			}
			output, err := engine.RunString(readAllString(stdin))
			if err != nil {
				return nil, fmt.Errorf("sed: %w", err)
			}
			result.WriteString(output)
		} else {
			cwd := grasp.Env(ctx, "PWD")
			if cwd == "" {
				cwd = "/"
			}

			for _, file := range files {
				resolvedPath := resolvePath(cwd, file)
				reader, err := v.Open(ctx, resolvedPath)
				if err != nil {
					return nil, fmt.Errorf("sed: can't read %s: %w", file, err)
				}
				content, err := io.ReadAll(reader)
				reader.Close()
				if err != nil {
					return nil, fmt.Errorf("sed: can't read %s: %w", file, err)
				}

				output, err := engine.RunString(string(content))
				if err != nil {
					return nil, fmt.Errorf("sed: %w", err)
				}
				result.WriteString(output)
			}
		}

		return io.NopCloser(strings.NewReader(result.String())), nil
	}
}

func sedInPlace(v *grasp.VirtualOS, engine *sed.Engine, files []string, ctx context.Context) (io.ReadCloser, error) {
	cwd := grasp.Env(ctx, "PWD")
	if cwd == "" {
		cwd = "/"
	}

	var result strings.Builder

	for _, file := range files {
		resolvedPath := resolvePath(cwd, file)

		// Read original content
		reader, err := v.Open(ctx, resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("sed: can't read %s: %w", file, err)
		}
		content, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return nil, fmt.Errorf("sed: can't read %s: %w", file, err)
		}

		// Process with sed
		output, err := engine.RunString(string(content))
		if err != nil {
			return nil, fmt.Errorf("sed: %w", err)
		}

		// Write back to file
		err = v.Write(ctx, resolvedPath, strings.NewReader(output))
		if err != nil {
			return nil, fmt.Errorf("sed: can't write %s: %w", file, err)
		}
	}

	return io.NopCloser(strings.NewReader(result.String())), nil
}

func parseSedArgs(args []string, opts *sedOpts) (files []string, err error) {
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-h", "--help":
			return nil, fmt.Errorf(`sed — stream editor for filtering and transforming text
Usage: sed [OPTIONS] -e SCRIPT [FILE]...
       sed [OPTIONS] -f SCRIPTFILE [FILE]...
Options:
  -n, --quiet, --silent  Suppress automatic printing of pattern space
  -e, --expression=SCRIPT Add the commands in SCRIPT to the set of commands
  -f, --file=SCRIPTFILE  Add the contents of SCRIPTFILE to the set of commands
  -i, --in-place         Edit files in place
`)
		case "-n", "--quiet", "--silent":
			opts.quiet = true
		case "-e", "--expression":
			if i+1 < len(args) {
				i++
				if opts.expr != "" {
					opts.expr += "; " + args[i]
				} else {
					opts.expr = args[i]
				}
			} else {
				return nil, fmt.Errorf("sed: option requires an argument: %s", args[i])
			}
		case "-f", "--file":
			if i+1 < len(args) {
				i++
				opts.file = args[i]
			} else {
				return nil, fmt.Errorf("sed: option requires an argument: %s", args[i])
			}
		case "-i", "--in-place":
			opts.inPlace = true
		default:
			if strings.HasPrefix(args[i], "-e") && len(args[i]) > 2 {
				// -eSCRIPT format
				if opts.expr != "" {
					opts.expr += "; " + args[i][2:]
				} else {
					opts.expr = args[i][2:]
				}
			} else if strings.HasPrefix(args[i], "-f") && len(args[i]) > 2 {
				// -fSCRIPTFILE format
				opts.file = args[i][2:]
			} else if strings.HasPrefix(args[i], "--expression=") {
				if opts.expr != "" {
					opts.expr += "; " + args[i][13:]
				} else {
					opts.expr = args[i][13:]
				}
			} else if strings.HasPrefix(args[i], "--file=") {
				opts.file = args[i][7:]
			} else if strings.HasPrefix(args[i], "-") && len(args[i]) > 1 {
				// Check for combined flags like -ni
				combinedFlags := args[i][1:]
				validCombined := true
				for j, c := range combinedFlags {
					switch c {
					case 'n':
						opts.quiet = true
					case 'i':
						opts.inPlace = true
					case 'e':
						// -e must be the last flag and needs an argument
						if j == len(combinedFlags)-1 && i+1 < len(args) {
							i++
							if opts.expr != "" {
								opts.expr += "; " + args[i]
							} else {
								opts.expr = args[i]
							}
						} else {
							validCombined = false
						}
					case 'f':
						// -f must be the last flag and needs an argument
						if j == len(combinedFlags)-1 && i+1 < len(args) {
							i++
							opts.file = args[i]
						} else {
							validCombined = false
						}
					default:
						validCombined = false
					}
				}
				if !validCombined {
					return nil, fmt.Errorf("sed: unknown option: %s", args[i])
				}
			} else {
				// Non-flag argument: could be script or file
				if opts.expr == "" && opts.file == "" && !strings.HasPrefix(args[i], "-") {
					// First non-option without -e/-f is treated as script
					opts.expr = args[i]
				} else {
					files = append(files, args[i])
				}
			}
		}
		i++
	}
	return files, nil
}

func readAllString(r io.Reader) string {
	content, err := io.ReadAll(r)
	if err != nil {
		return ""
	}
	return string(content)
}
