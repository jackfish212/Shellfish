package types

import (
	"fmt"
	"time"
)

// Entry represents a file, directory, or executable in the virtual filesystem.
type Entry struct {
	Name     string            // base name
	Path     string            // full path within shellfish
	IsDir    bool              // true if directory
	Perm     Perm              // permission bits
	Size     int64             // size in bytes (0 for dirs / executables)
	MimeType string            // MIME type hint
	Modified time.Time         // last modification time
	Meta     map[string]string // extensible metadata (e.g. "kind"="tool"|"prompt")
}

// String returns a formatted ls-style line for this entry.
func (e Entry) String() string {
	dirFlag := "-"
	name := e.Name
	if e.IsDir {
		dirFlag = "d"
		name += "/"
	}
	kind := ""
	if k, ok := e.Meta["kind"]; ok {
		kind = fmt.Sprintf(" [%s]", k)
	}
	return fmt.Sprintf("%s%s%s  %s", dirFlag, e.Perm, kind, name)
}
