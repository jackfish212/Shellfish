package types

import "time"

// WatchEvent describes a filesystem change, modeled after Linux inotify.
type WatchEvent struct {
	Type    EventType
	Path    string
	OldPath string // set only for EventRename
	Time    time.Time
}

// EventType is a bitmask of filesystem event kinds.
type EventType uint32

const (
	EventCreate EventType = 1 << iota
	EventWrite
	EventRemove
	EventRename
	EventMkdir

	EventAll EventType = EventCreate | EventWrite | EventRemove | EventRename | EventMkdir
)

func (e EventType) String() string {
	names := []struct {
		bit  EventType
		name string
	}{
		{EventCreate, "CREATE"},
		{EventWrite, "WRITE"},
		{EventRemove, "REMOVE"},
		{EventRename, "RENAME"},
		{EventMkdir, "MKDIR"},
	}
	var parts []string
	for _, n := range names {
		if e&n.bit != 0 {
			parts = append(parts, n.name)
		}
	}
	if len(parts) == 0 {
		return "NONE"
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "|" + parts[i]
	}
	return result
}

// Matches reports whether the event type matches any bit in the mask.
func (e EventType) Matches(mask EventType) bool {
	return e&mask != 0
}
