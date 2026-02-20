package shellfish

import "testing"

func TestCleanPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "/"},
		{".", "/"},
		{"/", "/"},
		{"/foo", "/foo"},
		{"/foo/", "/foo"},
		{"/foo/bar", "/foo/bar"},
		{"/foo//bar", "/foo/bar"},
		{"/foo/../bar", "/bar"},
		{"foo", "/foo"},
		{"foo/bar", "/foo/bar"},
		{`foo\bar`, "/foo/bar"},
		{`\foo\bar\`, "/foo/bar"},
		{"/foo/./bar", "/foo/bar"},
	}
	for _, tt := range tests {
		got := CleanPath(tt.in)
		if got != tt.want {
			t.Errorf("CleanPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBaseName(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/", "/"},
		{"/foo", "foo"},
		{"/foo/bar", "bar"},
		{"/a/b/c", "c"},
	}
	for _, tt := range tests {
		got := baseName(tt.in)
		if got != tt.want {
			t.Errorf("baseName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
