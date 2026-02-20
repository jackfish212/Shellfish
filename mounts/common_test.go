package mounts

import (
	"testing"
)

func TestNormPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/", ""},
		{"/foo", "foo"},
		{"/foo/", "foo"},
		{"/foo/bar", "foo/bar"},
		{"/foo/bar/", "foo/bar"},
		{"foo", "foo"},
		{"foo/", "foo"},
		{"//multiple//slashes//", "/multiple//slashes/"}, // TrimPrefix only removes one leading "/"
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normPath(tt.input)
			if result != tt.expected {
				t.Errorf("normPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBaseName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "/"},
		{"/", "/"},
		{"/foo", "foo"},
		{"/foo/bar", "bar"},
		{"/foo/bar/baz.txt", "baz.txt"},
		{"foo", "foo"},
		{"foo/bar", "bar"},
		{"/trailing/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := baseName(tt.input)
			if result != tt.expected {
				t.Errorf("baseName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
