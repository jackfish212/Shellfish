package shellfish

import (
	"strings"
	"sync"
	"time"
)

// Watcher receives filesystem change events. Created by VirtualOS.Watch.
// Call Close when done to free resources.
type Watcher struct {
	ch     chan WatchEvent
	prefix string
	mask   EventType
	hub    *watchHub
	closed chan struct{}
	once   sync.Once
}

// Events returns the channel on which events are delivered.
func (w *Watcher) Events() <-chan WatchEvent {
	return w.ch
}

// Close unsubscribes the watcher and closes its event channel.
func (w *Watcher) Close() error {
	w.once.Do(func() {
		close(w.closed)
		w.hub.remove(w)
	})
	return nil
}

// watchHub is a publish/subscribe hub for filesystem events.
type watchHub struct {
	mu       sync.RWMutex
	watchers []*Watcher
}

func newWatchHub() *watchHub {
	return &watchHub{}
}

// watch creates a new Watcher that receives events matching mask for paths
// under prefix. An empty prefix watches all paths.
func (h *watchHub) watch(prefix string, mask EventType) *Watcher {
	w := &Watcher{
		ch:     make(chan WatchEvent, 64),
		prefix: CleanPath(prefix),
		mask:   mask,
		hub:    h,
		closed: make(chan struct{}),
	}
	h.mu.Lock()
	h.watchers = append(h.watchers, w)
	h.mu.Unlock()
	return w
}

func (h *watchHub) remove(w *Watcher) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i, x := range h.watchers {
		if x == w {
			h.watchers = append(h.watchers[:i], h.watchers[i+1:]...)
			break
		}
	}
}

// emit sends an event to all matching watchers (non-blocking).
func (h *watchHub) emit(evType EventType, path string) {
	h.emitRename(evType, path, "")
}

func (h *watchHub) emitRename(evType EventType, path, oldPath string) {
	ev := WatchEvent{
		Type:    evType,
		Path:    path,
		OldPath: oldPath,
		Time:    time.Now(),
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, w := range h.watchers {
		if !evType.Matches(w.mask) {
			continue
		}
		if w.prefix != "/" && !strings.HasPrefix(path, w.prefix) {
			continue
		}
		select {
		case w.ch <- ev:
		case <-w.closed:
		default:
			// channel full, drop event (back-pressure)
		}
	}
}
