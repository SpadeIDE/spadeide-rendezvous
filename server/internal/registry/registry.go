package registry

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Slot is one online agent registration. Multiple opaque keys (aid and/or
// blinded rids) may point at the same Slot / Conn.
type Slot struct {
	Keys      []string
	Pub       []byte // Ed25519 pub when aid prove was used; nil for rid-only
	Conn      *websocket.Conn
	WriteMu   sync.Mutex
	ExpiresAt time.Time
	RemoteIP  string
}

// Registry is an in-memory TTL map (I3).
type Registry struct {
	mu    sync.Mutex
	slots map[string]*Slot
	ttl   time.Duration
}

func New(ttl time.Duration) *Registry {
	return &Registry{slots: make(map[string]*Slot), ttl: ttl}
}

// Upsert registers s under s.Keys (and legacy single-key via Keys[0]).
func (r *Registry) Upsert(s *Slot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(s.Keys) == 0 {
		return
	}
	s.ExpiresAt = time.Now().Add(r.ttl)
	for _, k := range s.Keys {
		r.slots[k] = s
	}
}

func (r *Registry) Touch(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.slots[key]
	if !ok {
		return false
	}
	s.ExpiresAt = time.Now().Add(r.ttl)
	return true
}

func (r *Registry) Get(key string) (*Slot, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.slots[key]
	if !ok || time.Now().After(s.ExpiresAt) {
		if ok {
			r.deleteSlotLocked(s)
		}
		return nil, false
	}
	return s, true
}

// Remove drops every key for this slot if conn matches (or conn is nil).
func (r *Registry) Remove(key string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.slots[key]
	if !ok {
		return
	}
	if conn != nil && s.Conn != conn {
		return
	}
	r.deleteSlotLocked(s)
}

func (r *Registry) deleteSlotLocked(s *Slot) {
	for _, k := range s.Keys {
		if cur, ok := r.slots[k]; ok && cur == s {
			delete(r.slots, k)
		}
	}
}

func (r *Registry) Sweep() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	n := 0
	seen := map[*Slot]struct{}{}
	for _, s := range r.slots {
		if _, ok := seen[s]; ok {
			continue
		}
		if now.After(s.ExpiresAt) {
			seen[s] = struct{}{}
			r.deleteSlotLocked(s)
			n++
		}
	}
	return n
}

func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[*Slot]struct{}{}
	for _, s := range r.slots {
		seen[s] = struct{}{}
	}
	return len(seen)
}

// CountByIP counts distinct connections from ip (not per-key aliases).
func (r *Registry) CountByIP(ip string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	seen := map[*websocket.Conn]struct{}{}
	for _, s := range r.slots {
		if now.After(s.ExpiresAt) {
			continue
		}
		if s.RemoteIP == ip {
			seen[s.Conn] = struct{}{}
		}
	}
	return len(seen)
}
