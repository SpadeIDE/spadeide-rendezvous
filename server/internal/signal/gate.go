package signal

import (
	"net/http"

	"github.com/spadeide/spade-rendezvous/internal/ratelimit"
	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

// admit runs the shared HTTP admission preamble (B2).
// Returns the client IP and whether the request may proceed.
func admit(w http.ResponseWriter, r *http.Request, lim *ratelimit.Limiter) (ip string, ok bool) {
	ip = wsutil.ClientIP(r)
	if lim != nil && !lim.Admit(w, ip) {
		return ip, false
	}
	return ip, true
}
