package shell

import (
	"testing"
)

func TestHasGlobChars(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"hello", false},
		{"*.txt", true},
		{"file?.txt", true},
		{"[abc]", true},
		{"no[glob", true}, // contains '[' which is a glob character
		{"", false},
		{"*", true},
		{"?", true},
		{"file[0-9].txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := hasGlobChars(tt.input)
			if result != tt.expected {
				t.Errorf("hasGlobChars(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
