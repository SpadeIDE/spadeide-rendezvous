package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// Counters are aggregate-only — never log handles, IPs, or payloads.
type Counters struct {
	AgentsOnline  atomic.Int64
	SessionsStart atomic.Int64
	Lookups       atomic.Int64
	LookupsAbsent atomic.Int64
	RelayBytes    atomic.Int64
	BansActive    atomic.Int64
	BanStrikes    atomic.Int64
	BanRejected   atomic.Int64
	BanEvictions  atomic.Int64
}

func (c *Counters) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(w, "spade_rv_agents_online %d\n", c.AgentsOnline.Load())
		_, _ = fmt.Fprintf(w, "spade_rv_sessions_started_total %d\n", c.SessionsStart.Load())
		_, _ = fmt.Fprintf(w, "spade_rv_lookups_total %d\n", c.Lookups.Load())
		_, _ = fmt.Fprintf(w, "spade_rv_lookups_absent_total %d\n", c.LookupsAbsent.Load())
		_, _ = fmt.Fprintf(w, "spade_rv_relay_bytes_total %d\n", c.RelayBytes.Load())
		_, _ = fmt.Fprintf(w, "spade_rv_bans_active %d\n", c.BansActive.Load())
		_, _ = fmt.Fprintf(w, "spade_rv_ban_strikes_total %d\n", c.BanStrikes.Load())
		_, _ = fmt.Fprintf(w, "spade_rv_ban_rejected_total %d\n", c.BanRejected.Load())
		_, _ = fmt.Fprintf(w, "spade_rv_ban_evictions_total %d\n", c.BanEvictions.Load())
	}
}
