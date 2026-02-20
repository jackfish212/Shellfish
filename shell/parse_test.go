package shell

import (
	"testing"
)

func TestParseRedirection(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantPath     string
		wantCmdPart  string
		wantNil      bool
	}{
		{
			name:        "simple redirect",
			input:       "echo hello > /tmp/file.txt",
			wantPath:    "/tmp/file.txt",
			wantCmdPart: "echo hello",
		},
		{
			name:        "redirect with trailing space",
			input:       "echo hello > /tmp/file.txt ",
			wantPath:    "/tmp/file.txt",
			wantCmdPart: "echo hello",
		},
		{
			name:        "redirect with trailing tab",
			input:       "echo hello > /tmp/file.txt\t",
			wantPath:    "/tmp/file.txt",
			wantCmdPart: "echo hello",
		},
		{
			name:        "redirect with multiple trailing spaces",
			input:       "echo hello > /tmp/file.txt   ",
			wantPath:    "/tmp/file.txt",
			wantCmdPart: "echo hello",
		},
		{
			name:        "append redirect",
			input:       "echo hello >> /tmp/file.txt",
			wantPath:    "/tmp/file.txt",
			wantCmdPart: "echo hello",
		},
		{
			name:        "append redirect with trailing space",
			input:       "echo hello >> /tmp/file.txt  ",
			wantPath:    "/tmp/file.txt",
			wantCmdPart: "echo hello",
		},
		{
			name:        "path with spaces in middle",
			input:       "echo hello > /tmp/my file.txt",
			wantPath:    "/tmp/my",
			wantCmdPart: "echo hello",
		},
		{
			name:        "path in quotes with trailing space",
			input:       `echo hello > "/tmp/file.txt" `,
			wantPath:    `"/tmp/file.txt"`,
			wantCmdPart: "echo hello",
		},
		{
			name:        "no redirection",
			input:       "echo hello",
			wantNil:     true,
		},
		{
			name:        "redirect with leading spaces after operator",
			input:       "echo hello >   /tmp/file.txt",
			wantPath:    "/tmp/file.txt",
			wantCmdPart: "echo hello",
		},
		{
			name:        "stderr redirect",
			input:       "cmd 2> /tmp/error.log",
			wantPath:    "/tmp/error.log",
			wantCmdPart: "cmd",
		},
		{
			name:        "stderr append redirect",
			input:       "cmd 2>> /tmp/error.log",
			wantPath:    "/tmp/error.log",
			wantCmdPart: "cmd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redir, cmdPart := parseRedirection(tt.input)

			if tt.wantNil {
				if redir != nil {
					t.Errorf("expected nil redirection, got path=%q", redir.path)
				}
				return
			}

			if redir == nil {
				t.Errorf("expected non-nil redirection, got nil")
				return
			}

			if redir.path != tt.wantPath {
				t.Errorf("path = %q, want %q", redir.path, tt.wantPath)
			}

			if cmdPart != tt.wantCmdPart {
				t.Errorf("cmdPart = %q, want %q", cmdPart, tt.wantCmdPart)
			}
		})
	}
}

func TestParseRedirectionTrailingSpaceInFilename(t *testing.T) {
	// This test specifically verifies the fix for the bug where trailing
	// spaces in redirect paths caused files to be created with spaces
	// in their names, making them unreadable via normal paths.

	// Simulate what LLM might generate
	input := "cat << 'EOF' > /shared/architect/suggestions.md "
	redir, cmdPart := parseRedirection(input)

	if redir == nil {
		t.Fatal("expected non-nil redirection")
	}

	// The path should NOT have trailing space after the fix
	if redir.path != "/shared/architect/suggestions.md" {
		t.Errorf("path = %q (has trailing space: %v), want %q",
			redir.path,
			len(redir.path) > 0 && redir.path[len(redir.path)-1] == ' ',
			"/shared/architect/suggestions.md")
	}

	if cmdPart != "cat << 'EOF'" {
		t.Errorf("cmdPart = %q, want %q", cmdPart, "cat << 'EOF'")
	}
}
