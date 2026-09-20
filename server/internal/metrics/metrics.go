package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// Counters are aggregate-only — never log handles, IPs, or payloads.
type Counters struct {
	AgentsOnline   atomic.Int64
	SessionsStart  atomic.Int64
	Lookups        atomic.Int64
	LookupsAbsent  atomic.Int64
	RelayBytes     atomic.Int64
}

func (c *Counters) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "spade_rv_agents_online %d\n", c.AgentsOnline.Load())
		fmt.Fprintf(w, "spade_rv_sessions_started_total %d\n", c.SessionsStart.Load())
		fmt.Fprintf(w, "spade_rv_lookups_total %d\n", c.Lookups.Load())
		fmt.Fprintf(w, "spade_rv_lookups_absent_total %d\n", c.LookupsAbsent.Load())
		fmt.Fprintf(w, "spade_rv_relay_bytes_total %d\n", c.RelayBytes.Load())
	}
}
