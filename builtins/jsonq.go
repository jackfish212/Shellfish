package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	grasp "github.com/jackfish212/grasp"
	"github.com/jackfish212/grasp/mounts"
	gojsonq "github.com/thedevsaddam/gojsonq/v2"
)

func builtinJsonq(v *grasp.VirtualOS) mounts.ExecFunc {
	return func(ctx context.Context, args []string, stdin io.Reader) (io.ReadCloser, error) {
		opts, queryPath, files, err := parseJsonqArgs(args)
		if err != nil {
			return nil, err
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
				return nil, fmt.Errorf("jsonq: no input")
			}
			output, err := executeQuery(stdin, queryPath, opts)
			if err != nil {
				return nil, fmt.Errorf("jsonq: %w", err)
			}
			result.WriteString(output)
			return io.NopCloser(strings.NewReader(result.String())), nil
		}

		// Process files
		for _, file := range files {
			resolvedPath := resolvePath(cwd, file)

			entry, err := v.Stat(ctx, resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("jsonq: %s: %w", file, err)
			}

			if entry.IsDir {
				return nil, fmt.Errorf("jsonq: %s: Is a directory", file)
			}

			reader, err := v.Open(ctx, resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("jsonq: %s: %w", file, err)
			}

			output, err := executeQuery(reader, queryPath, opts)
			reader.Close()
			if err != nil {
				return nil, fmt.Errorf("jsonq: %s: %w", file, err)
			}

			if len(files) > 1 {
				result.WriteString(file + ":\n")
			}
			result.WriteString(output)
		}

		return io.NopCloser(strings.NewReader(result.String())), nil
	}
}

type jsonqOpts struct {
	from          string // -f, --from path to start query from
	where         string // -w, --where condition (key op value)
	orWhere       string // --or-where condition
	whereIn       string // --where-in condition (key val1,val2,...)
	whereNil      string // --where-nil key
	whereNotNil   string // --where-not-nil key
	sortBy        string // --sort-by property
	sortOrder     string // --sort-order asc/desc
	groupBy       string // --group-by property
	distinct      string // --distinct property
	limit         int    // -n, --limit N
	offset        int    // --offset N
	pluck         string // --pluck property
	selectFields  string // -s, --select fields (comma separated)
	aggregate     string // --sum, --avg, --min, --max, --count
	aggregateProp string // property for aggregation
	raw           bool   // -r, --raw output raw value without JSON encoding
}

func parseJsonqArgs(args []string) (jsonqOpts, string, []string, error) {
	var opts jsonqOpts
	var queryPath string
	var files []string

	opts.limit = -1 // -1 means no limit
	opts.offset = -1

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			return opts, "", nil, fmt.Errorf(`jsonq — query JSON data using gojsonq
Usage: jsonq [OPTIONS] [QUERY] [FILE]...

QUERY is a dot-notation path to query (e.g., "items.[0].name")

Options:
  -f, --from PATH        Start query from path
  -w, --where COND       Where condition (e.g., "price > 100")
  --or-where COND        Or-where condition
  --where-in COND        Where-in condition (e.g., "id 1,2,3")
  --where-nil KEY        Where key is null
  --where-not-nil KEY    Where key is not null
  --sort-by PROP         Sort by property
  --sort-order ORDER     Sort order: asc (default) or desc
  --group-by PROP        Group by property
  --distinct PROP        Distinct by property
  -n, --limit N          Limit results to N items
  --offset N             Skip first N items
  --pluck PROP           Pluck property values
  -s, --select FIELDS    Select fields (comma separated)
  --sum PROP             Sum values of property
  --avg PROP             Average values of property
  --min PROP             Minimum value of property
  --max PROP             Maximum value of property
  --count                Count results
  -r, --raw              Output raw values without JSON encoding

Examples:
  jsonq "name.first" user.json
  jsonq --from items --where "price > 100" data.json
  jsonq --from items --sort-by price --sort-order desc data.json
  jsonq --from items --pluck name data.json
  cat data.json | jsonq "items.[0]"
`)
		case "-f", "--from":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --from requires a path argument")
			}
			opts.from = args[i+1]
			i++
		case "-w", "--where":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --where requires a condition argument")
			}
			opts.where = args[i+1]
			i++
		case "--or-where":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --or-where requires a condition argument")
			}
			opts.orWhere = args[i+1]
			i++
		case "--where-in":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --where-in requires a condition argument")
			}
			opts.whereIn = args[i+1]
			i++
		case "--where-nil":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --where-nil requires a key argument")
			}
			opts.whereNil = args[i+1]
			i++
		case "--where-not-nil":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --where-not-nil requires a key argument")
			}
			opts.whereNotNil = args[i+1]
			i++
		case "--sort-by":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --sort-by requires a property argument")
			}
			opts.sortBy = args[i+1]
			i++
		case "--sort-order":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --sort-order requires an order argument")
			}
			opts.sortOrder = args[i+1]
			i++
		case "--group-by":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --group-by requires a property argument")
			}
			opts.groupBy = args[i+1]
			i++
		case "--distinct":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --distinct requires a property argument")
			}
			opts.distinct = args[i+1]
			i++
		case "-n", "--limit":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --limit requires a number argument")
			}
			var limit int
			if _, err := fmt.Sscanf(args[i+1], "%d", &limit); err != nil {
				return opts, "", nil, fmt.Errorf("jsonq: invalid limit value: %s", args[i+1])
			}
			opts.limit = limit
			i++
		case "--offset":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --offset requires a number argument")
			}
			var offset int
			if _, err := fmt.Sscanf(args[i+1], "%d", &offset); err != nil {
				return opts, "", nil, fmt.Errorf("jsonq: invalid offset value: %s", args[i+1])
			}
			opts.offset = offset
			i++
		case "--pluck":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --pluck requires a property argument")
			}
			opts.pluck = args[i+1]
			i++
		case "-s", "--select":
			if i+1 >= len(args) {
				return opts, "", nil, fmt.Errorf("jsonq: --select requires fields argument")
			}
			opts.selectFields = args[i+1]
			i++
		case "--sum":
			opts.aggregate = "sum"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				opts.aggregateProp = args[i+1]
				i++
			}
		case "--avg":
			opts.aggregate = "avg"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				opts.aggregateProp = args[i+1]
				i++
			}
		case "--min":
			opts.aggregate = "min"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				opts.aggregateProp = args[i+1]
				i++
			}
		case "--max":
			opts.aggregate = "max"
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				opts.aggregateProp = args[i+1]
				i++
			}
		case "--count":
			opts.aggregate = "count"
		case "-r", "--raw":
			opts.raw = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return opts, "", nil, fmt.Errorf("jsonq: unknown option: %s", args[i])
			}
			// First non-flag argument without a known flag is the query path
			// Subsequent arguments are files
			if queryPath == "" && !fileExists(args[i]) {
				queryPath = args[i]
			} else {
				files = append(files, args[i])
			}
		}
	}

	return opts, queryPath, files, nil
}

// fileExists is a simple heuristic to detect if an argument is likely a file path
func fileExists(s string) bool {
	return strings.Contains(s, "/") || strings.Contains(s, "\\") ||
		strings.HasSuffix(s, ".json") || strings.HasSuffix(s, ".JSON")
}

func executeQuery(reader io.Reader, queryPath string, opts jsonqOpts) (string, error) {
	jq := gojsonq.New().Reader(reader)

	// Set starting path
	if opts.from != "" {
		jq.From(opts.from)
	} else if queryPath != "" && !isSimpleQuery(queryPath) {
		// If query path looks like a complex path, use From
		jq.From(queryPath)
		queryPath = ""
	}

	// Apply where conditions
	if opts.where != "" {
		key, op, val, err := parseWhereCondition(opts.where)
		if err != nil {
			return "", err
		}
		jq.Where(key, op, val)
	}

	if opts.orWhere != "" {
		key, op, val, err := parseWhereCondition(opts.orWhere)
		if err != nil {
			return "", err
		}
		jq.OrWhere(key, op, val)
	}

	if opts.whereIn != "" {
		key, vals, err := parseWhereInCondition(opts.whereIn)
		if err != nil {
			return "", err
		}
		jq.WhereIn(key, vals)
	}

	if opts.whereNil != "" {
		jq.WhereNil(opts.whereNil)
	}

	if opts.whereNotNil != "" {
		jq.WhereNotNil(opts.whereNotNil)
	}

	// Apply select fields
	if opts.selectFields != "" {
		fields := strings.Split(opts.selectFields, ",")
		for i, f := range fields {
			fields[i] = strings.TrimSpace(f)
		}
		jq.Select(fields...)
	}

	// Apply sorting
	if opts.sortBy != "" {
		if opts.sortOrder == "desc" {
			jq.SortBy(opts.sortBy, "desc")
		} else {
			jq.SortBy(opts.sortBy)
		}
	}

	// Apply grouping
	if opts.groupBy != "" {
		jq.GroupBy(opts.groupBy)
	}

	// Apply distinct
	if opts.distinct != "" {
		jq.Distinct(opts.distinct)
	}

	// Apply offset
	if opts.offset > 0 {
		jq.Offset(opts.offset)
	}

	// Apply limit
	if opts.limit > 0 {
		jq.Limit(opts.limit)
	}

	// Execute aggregation or pluck or get
	var result interface{}

	switch opts.aggregate {
	case "sum":
		if opts.aggregateProp != "" {
			result = jq.Sum(opts.aggregateProp)
		} else {
			result = jq.Sum()
		}
	case "avg":
		if opts.aggregateProp != "" {
			result = jq.Avg(opts.aggregateProp)
		} else {
			result = jq.Avg()
		}
	case "min":
		if opts.aggregateProp != "" {
			result = jq.Min(opts.aggregateProp)
		} else {
			result = jq.Min()
		}
	case "max":
		if opts.aggregateProp != "" {
			result = jq.Max(opts.aggregateProp)
		} else {
			result = jq.Max()
		}
	case "count":
		result = jq.Count()
	default:
		if opts.pluck != "" {
			result = jq.Pluck(opts.pluck)
		} else if queryPath != "" {
			result = jq.Find(queryPath)
		} else {
			result = jq.Get()
		}
	}

	// Check for errors
	if jq.Error() != nil {
		return "", jq.Error()
	}

	// Format output
	if opts.raw {
		return formatRaw(result), nil
	}

	return formatJSON(result)
}

func isSimpleQuery(q string) bool {
	// A simple query is a single-level path like "name" without dots
	return !strings.Contains(q, ".")
}

func parseWhereCondition(cond string) (string, string, interface{}, error) {
	// Parse conditions like "price > 100", "name = John", "id = 1"
	parts := strings.Fields(cond)
	if len(parts) < 3 {
		return "", "", nil, fmt.Errorf("invalid where condition: %s (expected 'key op value')", cond)
	}

	key := parts[0]
	op := parts[1]

	// Join remaining parts as value (handles values with spaces)
	valStr := strings.Join(parts[2:], " ")

	// Try to parse as number
	var val interface{}
	if strings.Contains(valStr, ".") {
		if f, err := parseFloat(valStr); err == nil {
			val = f
		} else {
			val = valStr
		}
	} else if i, err := parseInt(valStr); err == nil {
		val = i
	} else if valStr == "true" {
		val = true
	} else if valStr == "false" {
		val = false
	} else if valStr == "null" {
		val = nil
	} else {
		// Remove quotes if present
		val = strings.Trim(valStr, "\"'")
	}

	// Normalize operators
	switch op {
	case "==":
		op = "="
	case "!=":
		op = "!="
	}

	return key, op, val, nil
}

func parseWhereInCondition(cond string) (string, []interface{}, error) {
	// Parse conditions like "id 1,2,3" or "name a,b,c"
	// Split on first space only
	spaceIdx := strings.Index(cond, " ")
	if spaceIdx == -1 {
		return "", nil, fmt.Errorf("invalid where-in condition: %s (expected 'key val1,val2,...')", cond)
	}

	key := strings.TrimSpace(cond[:spaceIdx])
	valStrs := strings.Split(strings.TrimSpace(cond[spaceIdx+1:]), ",")

	var vals []interface{}
	for _, v := range valStrs {
		v = strings.TrimSpace(v)
		if strings.Contains(v, ".") {
			if f, err := parseFloat(v); err == nil {
				vals = append(vals, f)
				continue
			}
		}
		if i, err := parseInt(v); err == nil {
			vals = append(vals, i)
		} else {
			vals = append(vals, strings.Trim(v, "\"'"))
		}
	}

	return key, vals, nil
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}

func formatJSON(v interface{}) (string, error) {
	bytes, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes) + "\n", nil
}

func formatRaw(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val + "\n"
	case float64:
		// Check if it's a whole number
		if val == float64(int(val)) {
			return fmt.Sprintf("%d\n", int(val))
		}
		return fmt.Sprintf("%v\n", val)
	case int:
		return fmt.Sprintf("%d\n", val)
	case bool:
		return fmt.Sprintf("%t\n", val)
	case nil:
		return "null\n"
	case []interface{}:
		var sb strings.Builder
		for _, item := range val {
			sb.WriteString(formatRaw(item))
		}
		return sb.String()
	default:
		// For other types, return JSON representation
		bytes, _ := json.Marshal(val)
		return string(bytes) + "\n"
	}
}
