package signal

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/spadeide/spade-rendezvous/internal/metrics"
	"github.com/spadeide/spade-rendezvous/internal/ratelimit"
	"github.com/spadeide/spade-rendezvous/internal/registry"
	"github.com/spadeide/spade-rendezvous/internal/ticket"
	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

var upgrader = wsutil.Upgrader()

type Handler struct {
	Reg            *registry.Registry
	Tickets        *ticket.Manager
	TTL            time.Duration
	MaxAgentsPerIP int
	Lookups        *ratelimit.Window
	Counters       *metrics.Counters

	mu      sync.Mutex
	pending map[string]chan offerAck // sid → waiting client lookup
}

type offerAck struct {
	direct string
}

type envelope map[string]any

func (h *Handler) ensurePending() {
	if h.pending == nil {
		h.pending = make(map[string]chan offerAck)
	}
}

func (h *Handler) Agent(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(wsutil.MaxControlMessage)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	hello, err := ParseControlMessage(raw)
	if err != nil || hello["t"] != "hello" {
		_ = conn.WriteJSON(envelope{"t": "error", "msg": "expected hello"})
		return
	}

	aid, _ := hello["aid"].(string)
	aid = strings.ToLower(strings.TrimSpace(aid))
	rids := parseRIDs(hello)
	if aid == "" && len(rids) == 0 {
		_ = conn.WriteJSON(envelope{"t": "error", "msg": "aid or rid required"})
		return
	}

	ip := clientIP(r)
	if h.MaxAgentsPerIP > 0 {
		probe := aid
		if probe == "" {
			probe = rids[0]
		}
		if existing, ok := h.Reg.Get(probe); !ok || existing.RemoteIP != ip {
			if h.Reg.CountByIP(ip) >= h.MaxAgentsPerIP {
				_ = conn.WriteJSON(envelope{"t": "error", "msg": "too many agents from this address"})
				return
			}
		}
	}

	nonce := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return
	}
	_ = conn.WriteJSON(envelope{"t": "challenge", "nonce": base64.StdEncoding.EncodeToString(nonce)})

	_, raw, err = conn.ReadMessage()
	if err != nil {
		return
	}
	prove, err := ParseControlMessage(raw)
	if err != nil || prove["t"] != "prove" {
		_ = conn.WriteJSON(envelope{"t": "error", "msg": "expected prove"})
		return
	}

	var pub []byte
	if aid != "" {
		pubB64, _ := prove["pub"].(string)
		sigB64, _ := prove["sig"].(string)
		var err error
		pub, err = base64.StdEncoding.DecodeString(pubB64)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			_ = conn.WriteJSON(envelope{"t": "error", "msg": "bad pub"})
			return
		}
		sig, err := base64.StdEncoding.DecodeString(sigB64)
		if err != nil {
			_ = conn.WriteJSON(envelope{"t": "error", "msg": "bad sig"})
			return
		}
		if !ed25519.Verify(ed25519.PublicKey(pub), nonce, sig) {
			_ = conn.WriteJSON(envelope{"t": "error", "msg": "bad signature"})
			return
		}
		if DeriveAID(pub) != aid {
			_ = conn.WriteJSON(envelope{"t": "error", "msg": "aid mismatch"})
			return
		}
	}

	if len(rids) > 0 {
		macB64, _ := prove["mac"].(string)
		mac, err := base64.StdEncoding.DecodeString(macB64)
		if err != nil || len(mac) != sha256.Size {
			_ = conn.WriteJSON(envelope{"t": "error", "msg": "bad mac"})
			return
		}
		// Slot auth for blinded handles is knowledge of the unguessable rid
		// (HMAC of rvSecret). The mac binds this WS to the challenge without
		// presenting Ed25519 — the rendezvous never holds rvSecret, so it
		// cannot recompute the MAC; length/format is checked only.
		_ = mac
	}

	keys := make([]string, 0, 1+len(rids))
	if aid != "" {
		keys = append(keys, aid)
	}
	keys = append(keys, rids...)

	slot := &registry.Slot{
		Keys:     keys,
		Pub:      pub,
		Conn:     conn,
		RemoteIP: ip,
	}
	h.Reg.Upsert(slot)
	defer h.Reg.Remove(keys[0], conn)
	_ = conn.WriteJSON(envelope{"t": "ready", "ttl": int(h.TTL.Seconds())})

	primary := keys[0]
	for {
		_ = conn.SetReadDeadline(time.Now().Add(h.TTL + time.Minute))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		msg, err := ParseControlMessage(raw)
		if err != nil {
			continue
		}
		switch msg["t"] {
		case "ping":
			h.Reg.Touch(primary)
			_ = writeJSON(conn, &slot.WriteMu, envelope{"t": "pong"})
		case "offer-ack":
			sid, _ := msg["sid"].(string)
			direct, _ := msg["direct"].(string)
			h.mu.Lock()
			ch := h.pending[sid]
			h.mu.Unlock()
			if ch != nil {
				select {
				case ch <- offerAck{direct: direct}:
				default:
				}
			}
		}
	}
}

func parseRIDs(hello envelope) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if rid, ok := hello["rid"].(string); ok {
		add(rid)
	}
	switch v := hello["rids"].(type) {
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				add(s)
			}
		}
	case []string:
		for _, s := range v {
			add(s)
		}
	}
	return out
}

func (h *Handler) Client(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(wsutil.MaxControlMessage)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	if h.Lookups != nil && !h.Lookups.Allow(clientIP(r)) {
		_ = conn.WriteJSON(envelope{"t": "error", "msg": "rate limited"})
		return
	}
	if h.Counters != nil {
		h.Counters.Lookups.Add(1)
	}

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	lookup, err := ParseControlMessage(raw)
	if err != nil || lookup["t"] != "lookup" {
		_ = conn.WriteJSON(envelope{"t": "error", "msg": "expected lookup"})
		return
	}
	key, _ := lookup["rid"].(string)
	if key == "" {
		key, _ = lookup["aid"].(string)
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		_ = conn.WriteJSON(envelope{"t": "error", "msg": "aid or rid required"})
		return
	}
	mode, _ := lookup["mode"].(string)
	if mode != "punch" {
		mode = "full"
	}
	slot, ok := h.Reg.Get(key)
	if !ok {
		if h.Counters != nil {
			h.Counters.LookupsAbsent.Add(1)
		}
		_ = conn.WriteJSON(envelope{"t": "absent"})
		return
	}

	sid := uuid.NewString()
	agentTicket, err := h.Tickets.Issue(sid, "agent", 30*time.Second)
	if err != nil {
		_ = conn.WriteJSON(envelope{"t": "error", "msg": "ticket"})
		return
	}
	clientTicket, err := h.Tickets.Issue(sid, "client", 30*time.Second)
	if err != nil {
		_ = conn.WriteJSON(envelope{"t": "error", "msg": "ticket"})
		return
	}

	ackCh := make(chan offerAck, 1)
	h.mu.Lock()
	h.ensurePending()
	h.pending[sid] = ackCh
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, sid)
		h.mu.Unlock()
	}()

	_ = writeJSON(slot.Conn, &slot.WriteMu, envelope{
		"t": "offer", "sid": sid, "ticket": agentTicket, "mode": mode,
	})

	direct := ""
	select {
	case ack := <-ackCh:
		direct = ack.direct
	case <-time.After(4 * time.Second):
		// Agent slow or reflex failed — peer still works via relay only.
	}

	peer := envelope{"t": "peer", "sid": sid, "ticket": clientTicket}
	if direct != "" {
		peer["direct"] = direct
	}
	_ = conn.WriteJSON(peer)
}

func DeriveAID(pub []byte) string {
	sum := sha256.Sum256(pub)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
}

func writeJSON(c *websocket.Conn, mu *sync.Mutex, v any) error {
	mu.Lock()
	defer mu.Unlock()
	return c.WriteJSON(v)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
