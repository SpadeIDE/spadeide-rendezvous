package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerPrometheusText(t *testing.T) {
	c := &Counters{}
	c.AgentsOnline.Store(3)
	c.SessionsStart.Store(7)
	c.Lookups.Store(11)
	c.LookupsAbsent.Store(2)
	c.RelayBytes.Store(100)
	c.BansActive.Store(1)
	c.BanStrikes.Store(4)
	c.BanRejected.Store(9)
	c.BanEvictions.Store(5)

	rr := httptest.NewRecorder()
	c.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type %q", ct)
	}
	body := rr.Body.String()
	want := []string{
		"spade_rv_agents_online 3",
		"spade_rv_sessions_started_total 7",
		"spade_rv_lookups_total 11",
		"spade_rv_lookups_absent_total 2",
		"spade_rv_relay_bytes_total 100",
		"spade_rv_bans_active 1",
		"spade_rv_ban_strikes_total 4",
		"spade_rv_ban_rejected_total 9",
		"spade_rv_ban_evictions_total 5",
	}
	for _, line := range want {
		if !strings.Contains(body, line+"\n") {
			t.Fatalf("missing %q in:\n%s", line, body)
		}
	}
}
