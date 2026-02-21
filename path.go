package grasp

import (
	"path"
	"strings"
)

// CleanPath normalises an grasp path: forward-slashes, no trailing slash,
// always starts with "/".
func CleanPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	if p == "." || p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func baseName(p string) string {
	p = CleanPath(p)
	if p == "/" {
		return "/"
	}
	idx := strings.LastIndexByte(p, '/')
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}
