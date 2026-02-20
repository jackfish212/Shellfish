package shell

import (
	"testing"
)

func TestSplitPipe(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "no pipe",
			input:    "echo hello",
			expected: []string{"echo hello"},
		},
		{
			name:     "simple pipe",
			input:    "cat file | grep foo",
			expected: []string{"cat file ", " grep foo"},
		},
		{
			name:     "multiple pipes",
			input:    "cat file | grep foo | wc -l",
			expected: []string{"cat file ", " grep foo ", " wc -l"},
		},
		{
			name:     "pipe in single quotes",
			input:    "echo 'a|b' | cat",
			expected: []string{"echo 'a|b' ", " cat"},
		},
		{
			name:     "pipe in double quotes",
			input:    `echo "a|b" | cat`,
			expected: []string{`echo "a|b" `, " cat"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitPipe(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitPipe() = %v, want %v", result, tt.expected)
				return
			}
			for i, s := range result {
				if s != tt.expected[i] {
					t.Errorf("splitPipe()[%d] = %q, want %q", i, s, tt.expected[i])
				}
			}
		})
	}
}

func TestSplitLogicalOps(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []logicalSegment
	}{
		{
			name:     "no operator",
			input:    "echo hello",
			expected: []logicalSegment{{cmd: "echo hello", op: opNone}},
		},
		{
			name:     "and operator",
			input:    "true && echo success",
			expected: []logicalSegment{
				{cmd: "true ", op: opAnd},
				{cmd: " echo success", op: opNone},
			},
		},
		{
			name:     "or operator",
			input:    "false || echo fallback",
			expected: []logicalSegment{
				{cmd: "false ", op: opOr},
				{cmd: " echo fallback", op: opNone},
			},
		},
		{
			name:     "mixed operators",
			input:    "true && false || echo result",
			expected: []logicalSegment{
				{cmd: "true ", op: opAnd},
				{cmd: " false ", op: opOr},
				{cmd: " echo result", op: opNone},
			},
		},
		{
			name:     "operators in quotes",
			input:    "echo '&&' && echo test",
			expected: []logicalSegment{
				{cmd: "echo '&&' ", op: opAnd},
				{cmd: " echo test", op: opNone},
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLogicalOps(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitLogicalOps() = %v, want %v", result, tt.expected)
				return
			}
			for i, seg := range result {
				if seg.cmd != tt.expected[i].cmd || seg.op != tt.expected[i].op {
					t.Errorf("splitLogicalOps()[%d] = {cmd: %q, op: %v}, want {cmd: %q, op: %v}",
						i, seg.cmd, seg.op, tt.expected[i].cmd, tt.expected[i].op)
				}
			}
		})
	}
}

func TestSplitBySemicolon(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "no semicolon",
			input:    "echo hello",
			expected: []string{"echo hello"},
		},
		{
			name:     "simple semicolon",
			input:    "echo hello; echo world",
			expected: []string{"echo hello", " echo world"},
		},
		{
			name:     "multiple semicolons",
			input:    "a; b; c",
			expected: []string{"a", " b", " c"},
		},
		{
			name:     "semicolon in single quotes",
			input:    "echo 'a;b'; echo test",
			expected: []string{"echo 'a;b'", " echo test"},
		},
		{
			name:     "semicolon in double quotes",
			input:    `echo "a;b"; echo test`,
			expected: []string{`echo "a;b"`, " echo test"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "trailing semicolon",
			input:    "echo hello;",
			expected: []string{"echo hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitBySemicolon(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitBySemicolon() = %v, want %v", result, tt.expected)
				return
			}
			for i, s := range result {
				if s != tt.expected[i] {
					t.Errorf("splitBySemicolon()[%d] = %q, want %q", i, s, tt.expected[i])
				}
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple command",
			input:    "echo hello",
			expected: []string{"echo", "hello"},
		},
		{
			name:     "multiple spaces",
			input:    "echo   hello   world",
			expected: []string{"echo", "hello", "world"},
		},
		{
			name:     "single quotes",
			input:    "echo 'hello world'",
			expected: []string{"echo", "hello world"},
		},
		{
			name:     "double quotes",
			input:    `echo "hello world"`,
			expected: []string{"echo", "hello world"},
		},
		{
			name:     "mixed quotes",
			input:    `echo 'single' "double" bare`,
			expected: []string{"echo", "single", "double", "bare"},
		},
		{
			name:     "tabs",
			input:    "echo\thello\tworld",
			expected: []string{"echo", "hello", "world"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenize(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("tokenize() = %v, want %v", result, tt.expected)
				return
			}
			for i, s := range result {
				if s != tt.expected[i] {
					t.Errorf("tokenize()[%d] = %q, want %q", i, s, tt.expected[i])
				}
			}
		})
	}
}

func TestTokenizeWithQuoteInfo(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedTok   []string
		expectedQuote []bool
	}{
		{
			name:          "no quotes",
			input:         "echo hello",
			expectedTok:   []string{"echo", "hello"},
			expectedQuote: []bool{false, false},
		},
		{
			name:          "single quoted",
			input:         "echo 'hello'",
			expectedTok:   []string{"echo", "hello"},
			expectedQuote: []bool{false, true},
		},
		{
			name:          "double quoted",
			input:         `echo "hello"`,
			expectedTok:   []string{"echo", "hello"},
			expectedQuote: []bool{false, true},
		},
		{
			name:          "partially quoted",
			input:         `echo hel"lo"`,
			expectedTok:   []string{"echo", "hello"},
			expectedQuote: []bool{false, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, quoted := tokenizeWithQuoteInfo(tt.input)
			if len(tokens) != len(tt.expectedTok) {
				t.Errorf("tokens = %v, want %v", tokens, tt.expectedTok)
				return
			}
			if len(quoted) != len(tt.expectedQuote) {
				t.Errorf("quoted = %v, want %v", quoted, tt.expectedQuote)
				return
			}
			for i, tok := range tokens {
				if tok != tt.expectedTok[i] {
					t.Errorf("tokens[%d] = %q, want %q", i, tok, tt.expectedTok[i])
				}
				if quoted[i] != tt.expectedQuote[i] {
					t.Errorf("quoted[%d] = %v, want %v", i, quoted[i], tt.expectedQuote[i])
				}
			}
		})
	}
}

func TestFilterRedirectionArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no redirection",
			input:    []string{"echo", "hello"},
			expected: []string{"echo", "hello"},
		},
		{
			name:     "output redirect",
			input:    []string{"echo", "hello", ">", "file.txt"},
			expected: []string{"echo", "hello"},
		},
		{
			name:     "append redirect",
			input:    []string{"echo", "hello", ">>", "file.txt"},
			expected: []string{"echo", "hello"},
		},
		{
			name:     "stderr redirect",
			input:    []string{"cmd", "2>", "error.log"},
			expected: []string{"cmd"},
		},
		{
			name:     "stderr append redirect",
			input:    []string{"cmd", "2>>", "error.log"},
			expected: []string{"cmd"},
		},
		{
			name:     "combined redirect",
			input:    []string{"cmd", "&>", "all.log"},
			expected: []string{"cmd"},
		},
		{
			name:     "combined append redirect",
			input:    []string{"cmd", "&>>", "all.log"},
			expected: []string{"cmd"},
		},
		{
			name:     "stderr to stdout",
			input:    []string{"cmd", "2>&1"},
			expected: []string{"cmd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterRedirectionArgs(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("filterRedirectionArgs() = %v, want %v", result, tt.expected)
				return
			}
			for i, s := range result {
				if s != tt.expected[i] {
					t.Errorf("filterRedirectionArgs()[%d] = %q, want %q", i, s, tt.expected[i])
				}
			}
		})
	}
}

func TestFilterRedirectionArgsWithQuotes(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		quoted        []bool
		expectedArgs  []string
		expectedQuote []bool
	}{
		{
			name:          "no redirection",
			args:          []string{"echo", "hello"},
			quoted:        []bool{false, false},
			expectedArgs:  []string{"echo", "hello"},
			expectedQuote: []bool{false, false},
		},
		{
			name:          "output redirect",
			args:          []string{"echo", "hello", ">", "file.txt"},
			quoted:        []bool{false, true, false, false},
			expectedArgs:  []string{"echo", "hello"},
			expectedQuote: []bool{false, true},
		},
		{
			name:          "stderr to stdout",
			args:          []string{"cmd", "2>&1"},
			quoted:        []bool{false, false},
			expectedArgs:  []string{"cmd"},
			expectedQuote: []bool{false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, quoted := filterRedirectionArgsWithQuotes(tt.args, tt.quoted)
			if len(args) != len(tt.expectedArgs) {
				t.Errorf("args = %v, want %v", args, tt.expectedArgs)
				return
			}
			if len(quoted) != len(tt.expectedQuote) {
				t.Errorf("quoted = %v, want %v", quoted, tt.expectedQuote)
				return
			}
			for i, arg := range args {
				if arg != tt.expectedArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.expectedArgs[i])
				}
				if quoted[i] != tt.expectedQuote[i] {
					t.Errorf("quoted[%d] = %v, want %v", i, quoted[i], tt.expectedQuote[i])
				}
			}
		})
	}
}

func TestParseStderrToStdout(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedCmd string
		expectedOk  bool
	}{
		{
			name:        "with 2>&1",
			input:       "cmd 2>&1",
			expectedCmd: "cmd",
			expectedOk:  true,
		},
		{
			name:        "with 2>&1 and trailing space",
			input:       "cmd 2>&1 ",
			expectedCmd: "cmd",
			expectedOk:  true,
		},
		{
			name:        "without 2>&1",
			input:       "cmd",
			expectedCmd: "cmd",
			expectedOk:  false,
		},
		{
			name:        "2>&1 in middle",
			input:       "cmd 2>&1 | grep",
			expectedCmd: "cmd 2>&1 | grep",
			expectedOk:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, ok := parseStderrToStdout(tt.input)
			if cmd != tt.expectedCmd {
				t.Errorf("cmd = %q, want %q", cmd, tt.expectedCmd)
			}
			if ok != tt.expectedOk {
				t.Errorf("ok = %v, want %v", ok, tt.expectedOk)
			}
		})
	}
}
