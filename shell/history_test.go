package shell

import (
	"testing"
)

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		name     string
		entry    string
		expected string
	}{
		{
			name:     "entry with timestamp",
			entry:    "echo hello ## 2024-01-15T10:30:00Z",
			expected: "echo hello",
		},
		{
			name:     "entry without timestamp",
			entry:    "echo hello",
			expected: "echo hello",
		},
		{
			name:     "entry with multiple ##",
			entry:    "echo ## hello ## 2024-01-15T10:30:00Z",
			expected: "echo", // ExtractCommand returns content before first " ## "
		},
		{
			name:     "empty entry",
			entry:    "",
			expected: "",
		},
		{
			name:     "only timestamp",
			entry:    " ## 2024-01-15T10:30:00Z",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractCommand(tt.entry)
			if result != tt.expected {
				t.Errorf("ExtractCommand(%q) = %q, want %q", tt.entry, result, tt.expected)
			}
		})
	}
}
