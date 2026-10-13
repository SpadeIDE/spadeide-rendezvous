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

// NewWindow builds a sliding window. limit <= 0 means unlimited (A9) —
// matches SPADE_RV_RATE_*=0 accepted by config.Validate.
func NewWindow(limit int, window time.Duration) *Window {
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
// Unlimited windows (limit <= 0) always allow and store nothing.
func (w *Window) Allow(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.limit <= 0 {
		return true
	}
	now := time.Now()
	cut := now.Add(-w.window)
	prev := w.hits[key]
	kept := prev[:0]
	for _, t := range prev {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= w.limit {
		if len(kept) == 0 {
			delete(w.hits, key)
		} else {
			w.hits[key] = kept
		}
		return false
	}
	w.hits[key] = append(kept, now)
	return true
}

// Sweep drops idle keys so a unique-IP flood cannot grow the map forever.
func (w *Window) Sweep(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cut := now.Add(-w.window)
	for key, times := range w.hits {
		kept := times[:0]
		for _, t := range times {
			if t.After(cut) {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(w.hits, key)
		} else {
			w.hits[key] = kept
		}
	}
}
