package ratelimit

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProgressiveBanLadder(t *testing.T) {
	b := NewBanbook()
	b.ladder = []time.Duration{
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
	}
	b.strikeDecay = time.Hour

	d1 := b.Strike("1.2.3.4")
	if d1 != time.Minute {
		t.Fatalf("strike1=%s want 1m", d1)
	}
	banned, until := b.Blocked("1.2.3.4")
	if !banned || until.Before(time.Now()) {
		t.Fatal("expected active ban after strike 1")
	}

	// Second strike while banned still escalates.
	d2 := b.Strike("1.2.3.4")
	if d2 != 5*time.Minute {
		t.Fatalf("strike2=%s want 5m", d2)
	}
	d3 := b.Strike("1.2.3.4")
	if d3 != 15*time.Minute {
		t.Fatalf("strike3=%s want 15m", d3)
	}
	// Cap at last ladder step.
	d4 := b.Strike("1.2.3.4")
	if d4 != 15*time.Minute {
		t.Fatalf("strike4=%s want cap 15m", d4)
	}
}

func TestNoteRejectedDoesNotEscalate(t *testing.T) {
	// A5: banned-traffic counters are metrics only.
	b := NewBanbook()
	b.ladder = []time.Duration{time.Minute, 5 * time.Minute}
	b.Strike("9.9.9.9")
	for i := 0; i < 100; i++ {
		b.NoteRejected("9.9.9.9")
	}
	e := b.entries["9.9.9.9"]
	if e.strikes != 1 {
		t.Fatalf("strikes=%d want 1 (no escalate on NoteRejected)", e.strikes)
	}
	if e.rejectedWhileBanned != 100 {
		t.Fatalf("rejectedWhileBanned=%d want 100", e.rejectedWhileBanned)
	}
	if b.RejectedWhileBanned() != 100 {
		t.Fatalf("metric=%d want 100", b.RejectedWhileBanned())
	}
}

func TestSoftStrikeCapsAtSoftLadder(t *testing.T) {
	b := NewBanbook()
	ip := "198.51.100.1"
	var last time.Duration
	for i := 0; i < 10; i++ {
		last = b.SoftStrike(ip)
	}
	if last != 15*time.Minute {
		t.Fatalf("soft cap=%s want 15m", last)
	}
	e := b.entries[ip]
	if e.strikes != 0 {
		t.Fatalf("auth strikes=%d want 0 after SoftStrike only", e.strikes)
	}
	if e.softStrikes < 10 {
		t.Fatalf("softStrikes=%d want >=10", e.softStrikes)
	}
	// Auth Strike still reaches the long ladder.
	d := b.Strike(ip)
	if d != time.Minute {
		t.Fatalf("first auth strike=%s want 1m", d)
	}
	_ = b.Strike(ip) // 2 → 5m
	_ = b.Strike(ip) // 3 → 15m
	d = b.Strike(ip) // 4 → 1h
	if d != time.Hour {
		t.Fatalf("auth rung4=%s want 1h", d)
	}
}

func TestLimiterSoftTripBans(t *testing.T) {
	bans := NewBanbook()
	bans.softLadder = []time.Duration{2 * time.Second}
	lim := &Limiter{
		Bans: bans,
		Soft: NewWindow(3, time.Minute),
	}
	ip := "10.0.0.1"
	for i := 0; i < 3; i++ {
		if !lim.Allow(ip) {
			t.Fatalf("allow %d should pass", i)
		}
	}
	if lim.Allow(ip) {
		t.Fatal("4th should soft-fail and SoftStrike")
	}
	banned, _ := bans.Blocked(ip)
	if !banned {
		t.Fatal("expected ban after soft trip")
	}
	if bans.entries[ip].strikes != 0 {
		t.Fatal("rate SoftStrike must not inflate auth strikes")
	}
	if lim.Allow(ip) {
		t.Fatal("banned IP must be rejected")
	}
}

func TestStrikeDecay(t *testing.T) {
	b := NewBanbook()
	b.ladder = []time.Duration{time.Millisecond}
	b.strikeDecay = 50 * time.Millisecond
	b.Strike("8.8.8.8")
	time.Sleep(60 * time.Millisecond)
	// Ban expired + decay → next strike is level 1 again.
	d := b.Strike("8.8.8.8")
	if d != time.Millisecond {
		t.Fatalf("after decay want first rung, got %s", d)
	}
	if b.entries["8.8.8.8"].strikes != 1 {
		t.Fatalf("strikes=%d want 1 after decay reset", b.entries["8.8.8.8"].strikes)
	}
}

func TestWindowSweep(t *testing.T) {
	w := NewWindow(10, 20*time.Millisecond)
	_ = w.Allow("a")
	_ = w.Allow("b")
	time.Sleep(30 * time.Millisecond)
	w.Sweep(time.Now())
	w.mu.Lock()
	n := len(w.hits)
	w.mu.Unlock()
	if n != 0 {
		t.Fatalf("hits left=%d want 0", n)
	}
}

func TestBanKeyIPv6Slash64(t *testing.T) {
	a := BanKey("2001:db8:1:2:aaaa::1")
	b := BanKey("2001:db8:1:2:bbbb::2")
	if a != b {
		t.Fatalf("same /64 must collapse: %q vs %q", a, b)
	}
	if !strings.HasSuffix(a, "/64") {
		t.Fatalf("want /64 suffix, got %q", a)
	}
	c := BanKey("2001:db8:1:3::1")
	if a == c {
		t.Fatal("different /64 must not collapse")
	}
	if BanKey("10.0.0.1") != "10.0.0.1" {
		t.Fatal("IPv4 must stay unchanged")
	}
}

func TestBanbookMaxKeysEviction(t *testing.T) {
	b := NewBanbook()
	b.maxKeys = 4
	b.ladder = []time.Duration{time.Hour}
	for i := 0; i < 4; i++ {
		b.Strike(fmt.Sprintf("10.0.0.%d", i+1))
	}
	if b.Len() != 4 {
		t.Fatalf("len=%d want 4", b.Len())
	}
	b.Strike("10.0.0.99")
	if b.Len() != 4 {
		t.Fatalf("after overflow len=%d want 4", b.Len())
	}
	if b.Evictions() < 1 {
		t.Fatal("expected eviction metric")
	}
}

func TestAdmitWrites429(t *testing.T) {
	bans := NewBanbook()
	bans.ladder = []time.Duration{time.Minute}
	bans.softLadder = []time.Duration{time.Minute}
	lim := &Limiter{Bans: bans, Soft: NewWindow(2, time.Minute)}
	bans.Strike("203.0.113.8")
	rr := httptest.NewRecorder()
	if lim.Admit(rr, "203.0.113.8") {
		t.Fatal("banned IP must not admit")
	}
	if rr.Code != 429 {
		t.Fatalf("status=%d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}

	lim2 := &Limiter{Bans: NewBanbook(), Soft: NewWindow(1, time.Minute)}
	rr2 := httptest.NewRecorder()
	if !lim2.Admit(rr2, "198.51.100.1") {
		t.Fatal("first admit should pass")
	}
	rr3 := httptest.NewRecorder()
	if lim2.Admit(rr3, "198.51.100.1") {
		t.Fatal("soft over-limit must 429")
	}
	if rr3.Code != 429 {
		t.Fatalf("soft status=%d", rr3.Code)
	}
}

func TestBanbookSweepDecay(t *testing.T) {
	// Time-based eviction (A3 companion to MaxKeys). Strike → age past
	// strikeDecay → Sweep drops the entry and clears bansActive.
	b := NewBanbook()
	b.ladder = []time.Duration{time.Minute}
	b.strikeDecay = time.Hour
	b.Strike("10.0.0.7")
	if b.BansActive() != 1 {
		t.Fatalf("BansActive=%d want 1", b.BansActive())
	}
	e := b.entries["10.0.0.7"]
	if e == nil {
		t.Fatal("missing entry")
	}
	// Ban expired but still inside decay → keep history, active count 0.
	e.until = time.Now().Add(-time.Second)
	e.lastStrike = time.Now().Add(-time.Minute)
	e.lastSeen = time.Now().Add(-time.Minute)
	b.Sweep(time.Now())
	if b.Len() != 1 {
		t.Fatalf("within decay len=%d want 1", b.Len())
	}
	if b.BansActive() != 0 {
		t.Fatalf("expired ban BansActive=%d want 0", b.BansActive())
	}
	// Past decay → evict.
	e.lastStrike = time.Now().Add(-2 * time.Hour)
	e.lastSeen = time.Now().Add(-2 * time.Hour)
	b.Sweep(time.Now())
	if b.Len() != 0 {
		t.Fatalf("after decay len=%d want 0", b.Len())
	}
	if b.BansActive() != 0 {
		t.Fatalf("after decay BansActive=%d want 0", b.BansActive())
	}
}

func TestWindowUnlimited(t *testing.T) {
	// A9: limit 0 means unlimited, not a silent fallback to 60.
	w := NewWindow(0, time.Minute)
	for i := 0; i < 200; i++ {
		if !w.Allow("any") {
			t.Fatalf("unlimited window rejected at hit %d", i)
		}
	}
	w.mu.Lock()
	n := len(w.hits)
	w.mu.Unlock()
	if n != 0 {
		t.Fatalf("unlimited window must not store hits, got %d", n)
	}
}

func TestReflexSoftDoesNotBanHTTP(t *testing.T) {
	// A2 regression: Soft-only reflex limiter must not Strike the HTTP Banbook.
	httpBans := NewBanbook()
	httpBans.ladder = []time.Duration{time.Hour}
	httpLim := &Limiter{
		Bans: httpBans,
		Soft: NewWindow(100, time.Minute),
	}
	reflexLim := &Limiter{
		Soft: NewWindow(2, time.Minute),
	}
	ip := "203.0.113.50"
	for i := 0; i < 10; i++ {
		_ = reflexLim.Allow(ip)
	}
	if !httpLim.Allow(ip) {
		t.Fatal("HTTP admission must still allow after reflex soft flood")
	}
	if banned, _ := httpBans.Blocked(ip); banned {
		t.Fatal("HTTP Banbook must not be struck by reflex Soft-only limiter")
	}
	if httpBans.StrikesTotal() != 0 {
		t.Fatalf("strikes=%d want 0", httpBans.StrikesTotal())
	}
}
