package relay

import (
	"io"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

// Hub pairs two websocket sides by session id and pumps bytes blindly.
type Hub struct {
	mu       sync.Mutex
	waiting  map[string]*slot
	maxBytes int64
	maxLife  time.Duration
	pairWait time.Duration
	onBytes  func(n int64)
}

// slot is the first arrival. We deliberately do *not* ReadMessage until the
// peer arrives: any early application bytes (TLS ClientHello) stay in the
// WebSocket buffer and are delivered by pumpPair. Reading-and-discarding
// (or racing a buffer flush) drops the handshake and nested TLS hangs.
type slot struct {
	conn   *websocket.Conn
	role   string
	paired chan struct{}
}

func New(maxBytes int64, maxLife time.Duration) *Hub {
	if maxLife <= 0 {
		maxLife = 24 * time.Hour
	}
	return &Hub{
		waiting:  make(map[string]*slot),
		maxBytes: maxBytes,
		maxLife:  maxLife,
		pairWait: 2 * time.Minute,
	}
}

// OnBytes sets an optional aggregate byte counter (metrics).
func (h *Hub) OnBytes(fn func(n int64)) {
	h.onBytes = fn
}

// Arrive registers one side. Role must be agent|client; the peer must be the
// other role. When both arrive, bytes are copied until close or lifetime cap.
func (h *Hub) Arrive(sid, role string, conn *websocket.Conn) {
	if role != "agent" && role != "client" {
		log.Printf("relay: bad role %q sid=%s", role, sid)
		_ = conn.Close()
		return
	}
	// Bound every frame before any ReadMessage — maxBytes alone is cumulative
	// and used to check only after a full allocation.
	conn.SetReadLimit(wsutil.MaxRelayMessage)

	h.mu.Lock()
	if existing, ok := h.waiting[sid]; ok {
		if existing.role == role {
			h.mu.Unlock()
			log.Printf("relay: duplicate role %s sid=%s", role, sid)
			_ = conn.Close()
			return
		}
		delete(h.waiting, sid)
		h.mu.Unlock()

		close(existing.paired)
		log.Printf("relay: paired sid=%s", sid)
		h.pumpPair(existing.conn, conn)
		return
	}

	s := &slot{
		conn:   conn,
		role:   role,
		paired: make(chan struct{}),
	}
	h.waiting[sid] = s
	h.mu.Unlock()
	log.Printf("relay: waiting sid=%s role=%s", sid, role)

	wait := h.pairWait
	if wait <= 0 {
		wait = 2 * time.Minute
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-s.paired:
		// Peer owns the pump; this Arrive returns.
		return
	case <-timer.C:
		h.mu.Lock()
		if h.waiting[sid] == s {
			delete(h.waiting, sid)
		}
		h.mu.Unlock()
		log.Printf("relay: wait timeout sid=%s", sid)
		_ = conn.Close()
		return
	}
}

func (h *Hub) pumpPair(a, b *websocket.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		pump(a, b, h.maxBytes, h.onBytes)
		done <- struct{}{}
	}()
	go func() {
		pump(b, a, h.maxBytes, h.onBytes)
		done <- struct{}{}
	}()

	timer := time.NewTimer(h.maxLife)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	_ = a.Close()
	_ = b.Close()
}

func pump(dst, src *websocket.Conn, maxBytes int64, onBytes func(n int64)) {
	var n int64
	for {
		// NextReader + LimitReader refuse oversized frames without a full
		// []byte allocation of the attacker's claimed size.
		mt, r, err := src.NextReader()
		if err != nil {
			return
		}
		limited := io.LimitReader(r, wsutil.MaxRelayMessage+1)
		data, err := io.ReadAll(limited)
		if err != nil {
			return
		}
		if int64(len(data)) > wsutil.MaxRelayMessage {
			return
		}
		n += int64(len(data))
		if onBytes != nil {
			onBytes(int64(len(data)))
		}
		if maxBytes > 0 && n > maxBytes {
			return
		}
		_ = dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if err := dst.WriteMessage(mt, data); err != nil {
			return
		}
	}
}
