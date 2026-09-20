package ratelimit

import (
	"sync"
	"time"
)

// Window is a simple per-key sliding window (lookups / agents-per-IP).
type Window struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

func NewWindow(limit int, window time.Duration) *Window {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	return &Window{
		limit:  limit,
		window: window,
		hits:   make(map[string][]time.Time),
	}
}

// Allow records a hit and returns false when over the limit.
func (w *Window) Allow(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	cut := now.Add(-w.window)
	kept := w.hits[key][:0]
	for _, t := range w.hits[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= w.limit {
		w.hits[key] = kept
		return false
	}
	w.hits[key] = append(kept, now)
	return true
}
