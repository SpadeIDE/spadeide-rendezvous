package signal

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

func (h *Handler) Client(w http.ResponseWriter, r *http.Request) {
	if _, ok := admit(w, r, h.Lookups); !ok {
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(wsutil.MaxControlMessage)
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	if h.Counters != nil {
		h.Counters.Lookups.Add(1)
	}

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var lookup LookupMsg
	if err := DecodeControl(raw, &lookup); err != nil || lookup.T != "lookup" {
		// A11: same class as missing aid/rid — error + close, no first-frame strike.
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "expected lookup"})
		return
	}
	key := strings.ToLower(strings.TrimSpace(lookup.RID))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(lookup.AID))
	}
	if key == "" {
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "aid or rid required"})
		return
	}
	mode := lookup.Mode
	if mode != "punch" {
		mode = "full"
	}
	slot, ok := h.Reg.Get(key)
	if !ok {
		if h.Counters != nil {
			h.Counters.LookupsAbsent.Add(1)
		}
		absent := AbsentMsg{T: "absent"}
		h.attachNoticeAbsent(&absent)
		_ = writeConn(conn, absent)
		return
	}

	sid := uuid.NewString()
	agentTicket, err := h.Tickets.Issue(sid, "agent", 30*time.Second)
	if err != nil {
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "ticket"})
		return
	}
	clientTicket, err := h.Tickets.Issue(sid, "client", 30*time.Second)
	if err != nil {
		_ = writeConn(conn, ErrorMsg{T: "error", Msg: "ticket"})
		return
	}

	// Buffer 2: empty offer-ack + late direct must both land if the agent is fast.
	ackCh := make(chan offerAck, 2)
	h.mu.Lock()
	h.ensurePending()
	h.pending[sid] = ackCh
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, sid)
		h.mu.Unlock()
	}()

	_ = writeJSON(slot.Conn, &slot.WriteMu, OfferMsg{
		T: "offer", SID: sid, Ticket: agentTicket, Mode: mode,
	})

	// Peer as soon as the agent acks (even with an empty direct). Waiting on
	// UDP reflex here used to block every remote connect for up to ~4s.
	direct := ""
	select {
	case ack := <-ackCh:
		direct = ack.direct
	case <-time.After(2 * time.Second):
		// Agent never acked — still hand out a relay ticket; dial may fail.
	}

	peer := PeerMsg{T: "peer", SID: sid, Ticket: clientTicket, Direct: direct}
	h.attachNoticePeer(&peer)
	_ = writeConn(conn, peer)

	// Optional late direct for clients that keep the socket open. Do not block
	// the common path — close promptly so lookup returns and Relay can dial.
	if direct == "" {
		select {
		case ack := <-ackCh:
			if ack.direct != "" {
				_ = writeConn(conn, DirectMsg{T: "direct", Direct: ack.direct})
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
}
