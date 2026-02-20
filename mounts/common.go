// Package mounts provides built-in Mount implementations for AFS.
package mounts

import "strings"

func normPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	return p
}

func baseName(p string) string {
	if p == "" || p == "/" {
		return "/"
	}
	idx := strings.LastIndexByte(p, '/')
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}
