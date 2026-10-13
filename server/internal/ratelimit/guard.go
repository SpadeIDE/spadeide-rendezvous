package ratelimit

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultLadder is progressive lockout for Strike (auth/protocol soft-window
// trips in signal.noteFault, and bad-ticket floods on /v1/relay).
// Strike 1 → 1m, 2 → 5m, 3 → 15m, 4 → 1h, 5+ → 24h.
var DefaultLadder = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	time.Hour,
	24 * time.Hour,
}

// SoftLadder caps rate-limit SoftStrike bans (A5). CGNAT pools must not
// climb to 1h/24h from ordinary soft trips alone.
var SoftLadder = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
}

// DefaultMaxKeys caps Banbook memory under unique-key floods (A3).
const DefaultMaxKeys = 10_000

// Banbook tracks per-IP progressive bans shared across HTTP endpoints.
// SoftStrike (rate) and Strike (auth/protocol after a soft window) are
// separate ladders. Do not share a Banbook with unauthenticated UDP (A2).
type Banbook struct {
	mu sync.Mutex

	ladder         []time.Duration
	softLadder     []time.Duration
	strikeDecay    time.Duration // clean period resets strikes
	maxKeys        int
	entries        map[string]*banEntry
	strikesTotal   atomic.Int64
	bansActive     atomic.Int64
	rejectedBanned atomic.Int64
	evictions      atomic.Int64
}

type banEntry struct {
	strikes             int // auth Strike rung
	softStrikes         int // SoftStrike rung
	until               time.Time
	lastStrike          time.Time
	rejectedWhileBanned int
	lastSeen            time.Time
}

func NewBanbook() *Banbook {
	return &Banbook{
		ladder:      append([]time.Duration(nil), DefaultLadder...),
		softLadder:  append([]time.Duration(nil), SoftLadder...),
		strikeDecay: 24 * time.Hour,
		maxKeys:     DefaultMaxKeys,
		entries:     make(map[string]*banEntry),
	}
}

// BanKey maps a remote address to the ban/rate-limit key. IPv4 is unchanged;
// IPv6 is collapsed to /64 so one VPS allocation is one entry (A3/A5).
func BanKey(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	v6 := parsed.To16()
	if v6 == nil {
		return ip
	}
	masked := make(net.IP, len(v6))
	copy(masked, v6)
	for i := 8; i < 16; i++ {
		masked[i] = 0
	}
	return masked.String() + "/64"
}

// Blocked reports whether key is currently banned and when it expires.
func (b *Banbook) Blocked(key string) (banned bool, until time.Time) {
	if key == "" || b == nil {
		return false, time.Time{}
	}
	key = BanKey(key)
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.entries[key]
	if e == nil {
		return false, time.Time{}
	}
	e.lastSeen = time.Now()
	if e.until.After(time.Now()) {
		return true, e.until
	}
	return false, time.Time{}
}

// NoteRejected counts a rejection while banned (metrics only).
// A5: does not escalate — CGNAT retries must not climb the ladder.
func (b *Banbook) NoteRejected(key string) {
	if key == "" || b == nil {
		return
	}
	key = BanKey(key)
	b.rejectedBanned.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.entries[key]
	if e == nil || !e.until.After(time.Now()) {
		return
	}
	e.lastSeen = time.Now()
	e.rejectedWhileBanned++
}

// SoftStrike escalates the soft (rate-limit) ladder, capped at SoftLadder.
func (b *Banbook) SoftStrike(key string) time.Duration {
	return b.strike(key, true)
}

// Strike escalates the full auth ladder (1h / 24h rungs reachable).
func (b *Banbook) Strike(key string) time.Duration {
	return b.strike(key, false)
}

func (b *Banbook) strike(key string, soft bool) time.Duration {
	if key == "" || b == nil {
		return 0
	}
	key = BanKey(key)
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	e := b.entries[key]
	if e == nil {
		b.ensureCapacityLocked(now)
		e = &banEntry{}
		b.entries[key] = e
	}
	// Long clean period → forget old strikes.
	if (e.strikes > 0 || e.softStrikes > 0) && now.Sub(e.lastStrike) > b.strikeDecay {
		e.strikes = 0
		e.softStrikes = 0
		e.rejectedWhileBanned = 0
	}
	return b.strikeLocked(e, soft, now)
}

// ensureCapacityLocked frees a slot when at maxKeys. Prefers decayed idle
// entries, then the entry whose ban expires soonest.
func (b *Banbook) ensureCapacityLocked(now time.Time) {
	if b.maxKeys <= 0 || len(b.entries) < b.maxKeys {
		return
	}
	for key, e := range b.entries {
		if e.until.After(now) {
			continue
		}
		if now.Sub(e.lastStrike) > b.strikeDecay && now.Sub(e.lastSeen) > b.strikeDecay {
			delete(b.entries, key)
			b.evictions.Add(1)
			if len(b.entries) < b.maxKeys {
				return
			}
		}
	}
	if len(b.entries) < b.maxKeys {
		return
	}
	var victim string
	var best time.Time
	first := true
	for key, e := range b.entries {
		cand := e.until
		if !cand.After(now) {
			cand = e.lastSeen
		}
		if first || cand.Before(best) {
			first = false
			best = cand
			victim = key
		}
	}
	if victim == "" {
		return
	}
	if e := b.entries[victim]; e != nil && e.until.After(now) {
		b.bansActive.Add(-1)
	}
	delete(b.entries, victim)
	b.evictions.Add(1)
}

func (b *Banbook) strikeLocked(e *banEntry, soft bool, now time.Time) time.Duration {
	wasBanned := e.until.After(now)
	e.lastStrike = now
	e.lastSeen = now
	e.rejectedWhileBanned = 0
	var ladder []time.Duration
	var idx int
	if soft {
		e.softStrikes++
		ladder = b.softLadder
		idx = e.softStrikes - 1
	} else {
		e.strikes++
		ladder = b.ladder
		idx = e.strikes - 1
	}
	if len(ladder) == 0 {
		ladder = SoftLadder
	}
	if idx >= len(ladder) {
		idx = len(ladder) - 1
	}
	dur := ladder[idx]
	e.until = now.Add(dur)
	b.strikesTotal.Add(1)
	if !wasBanned {
		b.bansActive.Add(1)
	}
	return dur
}

// Sweep drops expired idle entries.
func (b *Banbook) Sweep(now time.Time) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	active := int64(0)
	for key, e := range b.entries {
		if e.until.After(now) {
			active++
			continue
		}
		// Keep strike history until decay window passes.
		if now.Sub(e.lastStrike) > b.strikeDecay && now.Sub(e.lastSeen) > b.strikeDecay {
			delete(b.entries, key)
		}
	}
	b.bansActive.Store(active)
}

func (b *Banbook) StrikesTotal() int64 {
	if b == nil {
		return 0
	}
	return b.strikesTotal.Load()
}

func (b *Banbook) BansActive() int64 {
	if b == nil {
		return 0
	}
	return b.bansActive.Load()
}

func (b *Banbook) RejectedWhileBanned() int64 {
	if b == nil {
		return 0
	}
	return b.rejectedBanned.Load()
}

func (b *Banbook) Evictions() int64 {
	if b == nil {
		return 0
	}
	return b.evictions.Load()
}

// Len returns the number of tracked keys (test / metrics helper).
func (b *Banbook) Len() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

// Limiter pairs a soft per-minute window with a shared Banbook.
// Soft-only (Bans == nil) silently drops over-limit traffic without
// progressive bans — required for spoofable UDP reflex (A2).
type Limiter struct {
	Bans *Banbook
	Soft *Window
}

// Allow returns false if the key is banned or over the soft limit.
// Crossing the soft limit issues a SoftStrike when Bans is set.
func (l *Limiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	key = BanKey(key)
	if l.Bans != nil {
		if banned, _ := l.Bans.Blocked(key); banned {
			l.Bans.NoteRejected(key)
			return false
		}
	}
	if l.Soft != nil && !l.Soft.Allow(key) {
		if l.Bans != nil {
			l.Bans.SoftStrike(key)
		}
		return false
	}
	return true
}

// CheckBan is a cheap pre-check (no soft-limit hit recorded).
func (l *Limiter) CheckBan(key string) (banned bool, until time.Time) {
	if l == nil || l.Bans == nil {
		return false, time.Time{}
	}
	key = BanKey(key)
	banned, until = l.Bans.Blocked(key)
	if banned {
		l.Bans.NoteRejected(key)
	}
	return banned, until
}
