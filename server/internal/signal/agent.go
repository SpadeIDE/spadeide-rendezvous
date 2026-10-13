package signal

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spadeide/spade-rendezvous/internal/registry"
	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

func (h *Handler) Agent(w http.ResponseWriter, r *http.Request) {
	ip, ok := admit(w, r, h.Agents)
	if !ok {
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(wsutil.MaxControlMessage)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var hello HelloMsg
	if err := DecodeControl(raw, &hello); err != nil || hello.T != "hello" {
		h.noteFault(ip, h.Agents)
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "expected hello"})
		return
	}

	aid := strings.ToLower(strings.TrimSpace(hello.AID))
	rids := collectRIDs(hello)
	// A1(2)/§10: every registration must prove the host Ed25519 identity.
	// Rids are lookup aliases only — an unpaired iPad holding rvSecret cannot
	// register a ghost agent (it lacks the agent private key).
	if aid == "" {
		h.noteFault(ip, h.Agents)
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "aid required"})
		return
	}
	if h.MaxAgentsPerIP > 0 {
		if existing, ok := h.Reg.Get(aid); !ok || existing.RemoteIP != ip {
			if h.Reg.CountByIP(ip) >= h.MaxAgentsPerIP {
				_ = writeConn(conn, ErrorMsg{T: "error", Msg: "too many agents from this address"})
				return
			}
		}
	}

	nonce := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return
	}
	_ = writeConn(conn, ChallengeMsg{T: "challenge", Nonce: base64.StdEncoding.EncodeToString(nonce)})

	_, raw, err = conn.ReadMessage()
	if err != nil {
		return
	}
	var prove ProveMsg
	if err := DecodeControl(raw, &prove); err != nil || prove.T != "prove" {
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "expected prove"})
		return
	}

	pub, err := base64.StdEncoding.DecodeString(prove.Pub)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "bad pub"})
		return
	}
	sig, err := base64.StdEncoding.DecodeString(prove.Sig)
	if err != nil {
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "bad sig"})
		return
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), nonce, sig) {
		h.noteFault(ip, h.Agents)
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "bad signature"})
		return
	}
	if DeriveAID(pub) != aid {
		h.noteFault(ip, h.Agents)
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "aid mismatch"})
		return
	}

	if len(rids) > 0 {
		mac, err := base64.StdEncoding.DecodeString(prove.MAC)
		if err != nil || len(mac) != sha256.Size {
			_ = writeConn(conn, ErrorMsg{T: "error", Msg: "bad mac"})
			return
		}
		// Format gate only — server never holds rvSecret. Real authorisation
		// is the Ed25519 prove above; mac stays for wire compat with agents.
		_ = mac
	}

	keys := make([]string, 0, 1+len(rids))
	keys = append(keys, aid)
	keys = append(keys, rids...)

	slot := &registry.Slot{
		Keys:     keys,
		Pub:      pub,
		Conn:     conn,
		RemoteIP: ip,
	}
	// A1: do not let a second connection steal a live slot for the same rid/aid.
	if !h.Reg.Upsert(slot) {
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "slot taken"})
		return
	}
	defer h.Reg.Remove(keys[0], conn)
	// A4: every post-Upsert write must take WriteMu (lookups may offer concurrently).
	_ = writeJSON(conn, &slot.WriteMu, ReadyMsg{T: "ready", TTL: int(h.TTL.Seconds())})

	primary := keys[0]
	for {
		_ = conn.SetReadDeadline(time.Now().Add(h.TTL + time.Minute))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		t, err := PeekType(raw)
		if err != nil {
			continue
		}
		switch t {
		case "ping":
			h.Reg.Touch(primary)
			_ = writeJSON(conn, &slot.WriteMu, PongMsg{T: "pong"})
		case "offer-ack":
			var ack OfferAckMsg
			if err := DecodeControl(raw, &ack); err != nil {
				continue
			}
			h.mu.Lock()
			ch := h.pending[ack.SID]
			h.mu.Unlock()
			if ch != nil {
				select {
				case ch <- offerAck{direct: ack.Direct}:
				default:
				}
			}
		}
	}
}

func collectRIDs(hello HelloMsg) []string {
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
	add(hello.RID)
	for _, s := range hello.RIDs {
		add(s)
	}
	return out
}
