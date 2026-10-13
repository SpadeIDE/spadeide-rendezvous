package ratelimit

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// RejectHTTP writes 429 with Retry-After when the client is banned or limited.
func RejectHTTP(w http.ResponseWriter, until time.Time, banned bool) {
	msg := "rate limited"
	if banned {
		msg = "temporarily banned"
	}
	secs := int(time.Until(until).Seconds())
	if secs < 1 {
		secs = 60
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = fmt.Fprintf(w, "%s\n", msg)
}

// Admit reports whether ip may proceed, writing 429 itself when not (B2).
// Soft-nil limiters still honour Banbook checks.
func (l *Limiter) Admit(w http.ResponseWriter, ip string) bool {
	if l == nil {
		return true
	}
	if banned, until := l.CheckBan(ip); banned {
		RejectHTTP(w, until, true)
		return false
	}
	if !l.Allow(ip) {
		until := time.Now().Add(time.Minute)
		if l.Bans != nil {
			if b, u := l.Bans.Blocked(ip); b {
				until = u
			}
		}
		RejectHTTP(w, until, true)
		return false
	}
	return true
}
