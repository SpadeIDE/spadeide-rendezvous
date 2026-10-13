package server

import (
	"net/http"
	"time"

	"github.com/spadeide/spade-rendezvous/internal/config"
	"github.com/spadeide/spade-rendezvous/internal/metrics"
	"github.com/spadeide/spade-rendezvous/internal/notice"
	"github.com/spadeide/spade-rendezvous/internal/ratelimit"
	"github.com/spadeide/spade-rendezvous/internal/registry"
	"github.com/spadeide/spade-rendezvous/internal/relay"
	"github.com/spadeide/spade-rendezvous/internal/signal"
	"github.com/spadeide/spade-rendezvous/internal/ticket"
	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

// Wire holds the constructed rendezvous pieces (B4). Testable without TLS.
type Wire struct {
	Cfg      *config.Config
	Reg      *registry.Registry
	Tickets  *ticket.Manager
	Hub      *relay.Hub
	Bans     *ratelimit.Banbook
	Lookups  *ratelimit.Limiter
	Agents   *ratelimit.Limiter
	Reflex   *ratelimit.Limiter
	Notices  *notice.Store
	Sig      *signal.Handler
	Counters *metrics.Counters
	Mux      *http.ServeMux
}

// NewWire builds registry, tickets, limiters, signal handler, and HTTP mux.
func NewWire(cfg *config.Config) (*Wire, error) {
	reg := registry.New(cfg.AgentTTL)
	tickets, err := ticket.New()
	if err != nil {
		return nil, err
	}
	hub := relay.New(cfg.SessionMaxBytes, time.Duration(cfg.SessionMaxSecs)*time.Second)
	counters := &metrics.Counters{}
	hub.OnBytes(func(n int64) { counters.RelayBytes.Add(n) })

	bans := ratelimit.NewBanbook()
	lookups := &ratelimit.Limiter{
		Bans: bans,
		Soft: ratelimit.NewWindow(cfg.LookupsPerMin, time.Minute),
	}
	agents := &ratelimit.Limiter{
		Bans: bans,
		Soft: ratelimit.NewWindow(cfg.AgentPerMin, time.Minute),
	}
	// A2: soft window only — spoofable UDP must never Strike the HTTP Banbook.
	reflexLimit := &ratelimit.Limiter{
		Soft: ratelimit.NewWindow(cfg.ReflexPerMin, time.Minute),
	}
	// Relay admission: Banbook only (no soft window on successful ticket dials).
	relayGate := &ratelimit.Limiter{Bans: bans}

	notices := notice.New(cfg.NoticeFile)
	sig := &signal.Handler{
		Reg:            reg,
		Tickets:        tickets,
		TTL:            cfg.AgentTTL,
		MaxAgentsPerIP: cfg.MaxAgentsPerIP,
		Bans:           bans,
		Lookups:        lookups,
		Agents:         agents,
		Counters:       counters,
		Notice:         notices,
	}

	w := &Wire{
		Cfg:      cfg,
		Reg:      reg,
		Tickets:  tickets,
		Hub:      hub,
		Bans:     bans,
		Lookups:  lookups,
		Agents:   agents,
		Reflex:   reflexLimit,
		Notices:  notices,
		Sig:      sig,
		Counters: counters,
		Mux:      http.NewServeMux(),
	}

	w.Mux.HandleFunc("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("ok"))
	})
	w.Mux.HandleFunc("/v1/notice", notices.Handler())
	w.Mux.HandleFunc("/v1/agent", sig.Agent)
	w.Mux.HandleFunc("/v1/client", sig.Client)
	w.Mux.HandleFunc("/v1/relay", func(rw http.ResponseWriter, r *http.Request) {
		ip := wsutil.ClientIP(r)
		if !relayGate.Admit(rw, ip) {
			return
		}
		tkt := r.URL.Query().Get("ticket")
		parsed, err := tickets.Verify(tkt)
		if err != nil {
			// Probe flood of bad tickets — soft strike after many via agent window.
			if agents.Soft != nil && !agents.Soft.Allow("badticket:"+ip) {
				bans.Strike(ip)
			}
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			return
		}
		up := wsutil.Upgrader()
		conn, err := up.Upgrade(rw, r, nil)
		if err != nil {
			return
		}
		conn.SetReadLimit(wsutil.MaxRelayMessage)
		counters.SessionsStart.Add(1)
		hub.Arrive(parsed.SID, parsed.Role, conn)
	})

	return w, nil
}

// SweepOnce runs the periodic maintenance pass once (tests / ticker body).
func (w *Wire) SweepOnce(now time.Time) {
	if w == nil {
		return
	}
	w.Reg.Sweep()
	w.Tickets.Sweep(now)
	w.Notices.Refresh()
	w.Bans.Sweep(now)
	if w.Lookups.Soft != nil {
		w.Lookups.Soft.Sweep(now)
	}
	if w.Agents.Soft != nil {
		w.Agents.Soft.Sweep(now)
	}
	if w.Reflex.Soft != nil {
		w.Reflex.Soft.Sweep(now)
	}
	w.Counters.AgentsOnline.Store(int64(w.Reg.Len()))
	w.Counters.BansActive.Store(w.Bans.BansActive())
	w.Counters.BanStrikes.Store(w.Bans.StrikesTotal())
	w.Counters.BanRejected.Store(w.Bans.RejectedWhileBanned())
	w.Counters.BanEvictions.Store(w.Bans.Evictions())
}
