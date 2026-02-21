package builtins

import (
	"strings"

	grasp "github.com/jackfish212/grasp"
)

func hasFlag(args []string, flags ...string) bool {
	set := make(map[string]bool)
	for _, f := range flags {
		set[f] = true
	}
	for _, a := range args {
		if set[a] {
			return true
		}
	}
	return false
}

func resolvePath(cwd, p string) string {
	if strings.HasPrefix(p, "/") {
		return grasp.CleanPath(p)
	}
	p = strings.TrimPrefix(p, "./")
	if cwd == "" || cwd == "/" {
		return grasp.CleanPath("/" + p)
	}
	return grasp.CleanPath(cwd + "/" + p)
}

func parseLsFlags(args []string) (bool, bool, []string) {
	var showLong, showAll bool
	var filtered []string

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && arg != "-" && arg != "--" {
			flagContent := strings.TrimPrefix(arg, "-")
			for _, ch := range flagContent {
				switch ch {
				case 'l':
					showLong = true
				case 'a':
					showAll = true
				}
			}
		} else {
			filtered = append(filtered, arg)
		}
	}

	return showLong, showAll, filtered
}
