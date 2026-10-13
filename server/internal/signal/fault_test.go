package signal

import (
	"testing"
	"time"

	"github.com/spadeide/spade-rendezvous/internal/ratelimit"
)

func TestNoteFaultSingleDoesNotBan(t *testing.T) {
	bans := ratelimit.NewBanbook()
	lim := &ratelimit.Limiter{
		Bans: bans,
		Soft: ratelimit.NewWindow(3, time.Minute),
	}
	h := &Handler{Bans: bans, Agents: lim}
	ip := "203.0.113.9"
	h.noteFault(ip, lim)
	if banned, _ := bans.Blocked(ip); banned {
		t.Fatal("single protocol fault must not ban (A6)")
	}
	if bans.StrikesTotal() != 0 {
		t.Fatalf("strikes=%d want 0", bans.StrikesTotal())
	}
}

func TestNoteFaultSoftTripStrikes(t *testing.T) {
	bans := ratelimit.NewBanbook()
	lim := &ratelimit.Limiter{
		Bans: bans,
		Soft: ratelimit.NewWindow(2, time.Minute),
	}
	h := &Handler{Bans: bans, Agents: lim}
	ip := "203.0.113.10"
	h.noteFault(ip, lim) // 1 — under
	h.noteFault(ip, lim) // 2 — under
	if banned, _ := bans.Blocked(ip); banned {
		t.Fatal("still under soft window")
	}
	h.noteFault(ip, lim) // 3 — over → Strike
	if banned, _ := bans.Blocked(ip); !banned {
		t.Fatal("soft-window trip must Strike")
	}
}
