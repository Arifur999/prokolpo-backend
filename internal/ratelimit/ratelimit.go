// Package ratelimit counts events per key in a sliding window, in memory.
//
// State lives in this process, so limits are per instance; that holds while
// the API runs as one container. Move it to the database or Redis before
// scaling out.
package ratelimit

import (
	"sync"
	"time"
)

// pruneEvery bounds memory: every this many calls, keys with no recent events are dropped.
const pruneEvery = 1024

// Window allows at most limit events per key within the window.
type Window struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu    sync.Mutex
	hits  map[string][]time.Time
	calls int
}

// New returns a Window allowing limit events per key per window.
func New(limit int, window time.Duration) *Window {
	return &Window{limit: limit, window: window, now: time.Now, hits: map[string][]time.Time{}}
}

// Exceeded reports whether key has used up its events in the current window.
func (w *Window) Exceeded(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.recent(key)) >= w.limit
}

// Hit records one event for key.
func (w *Window) Hit(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hits[key] = append(w.recent(key), w.now())
	w.calls++
	if w.calls%pruneEvery == 0 {
		w.prune()
	}
}

// Allow records an event for key unless the key is over its limit.
func (w *Window) Allow(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.recent(key)) >= w.limit {
		return false
	}
	w.hits[key] = append(w.hits[key], w.now())
	return true
}

// Reset forgets key, for example after a successful login.
func (w *Window) Reset(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.hits, key)
}

// recent drops events older than the window and returns what is left. Callers hold mu.
func (w *Window) recent(key string) []time.Time {
	cutoff := w.now().Add(-w.window)
	events := w.hits[key]
	i := 0
	for i < len(events) && !events[i].After(cutoff) {
		i++
	}
	events = events[i:]
	if len(events) == 0 {
		delete(w.hits, key)
		return nil
	}
	w.hits[key] = events
	return events
}

func (w *Window) prune() {
	for key := range w.hits {
		w.recent(key)
	}
}
