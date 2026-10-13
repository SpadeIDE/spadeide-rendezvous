package signal

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/spadeide/spade-rendezvous/internal/metrics"
	"github.com/spadeide/spade-rendezvous/internal/notice"
	"github.com/spadeide/spade-rendezvous/internal/ratelimit"
	"github.com/spadeide/spade-rendezvous/internal/registry"
	"github.com/spadeide/spade-rendezvous/internal/ticket"
	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

var upgrader = wsutil.Upgrader()

// writeWait bounds every websocket write (A7). Matches relay.pump order of magnitude.
const writeWait = 10 * time.Second

type Handler struct {
	Reg            *registry.Registry
	Tickets        *ticket.Manager
	TTL            time.Duration
	MaxAgentsPerIP int
	// Shared progressive bans + per-endpoint soft windows.
	Bans     *ratelimit.Banbook
	Lookups  *ratelimit.Limiter
	Agents   *ratelimit.Limiter
	Counters *metrics.Counters
	// Optional operator announcement (nil = disabled).
	Notice *notice.Store

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

func (h *Handler) attachNoticePeer(peer *PeerMsg) {
	if h.Notice == nil || peer == nil {
		return
	}
	if n := h.Notice.Get(); n != nil {
		peer.Notice = n
	}
}

func (h *Handler) attachNoticeAbsent(absent *AbsentMsg) {
	if h.Notice == nil || absent == nil {
		return
	}
	if n := h.Notice.Get(); n != nil {
		absent.Notice = n
	}
}

// noteFault records a protocol/auth fault in lim's soft window.
// A single fault closes the socket with an error; only after the soft
// window trips do we Strike the full auth ladder (A6).
func (h *Handler) noteFault(ip string, lim *ratelimit.Limiter) {
	if ip == "" || lim == nil || lim.Soft == nil {
		return
	}
	if lim.Soft.Allow("proto:" + ratelimit.BanKey(ip)) {
		return
	}
	if h.Bans != nil {
		h.Bans.Strike(ip)
	}
}

func DeriveAID(pub []byte) string {
	sum := sha256.Sum256(pub)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
}

func writeConn(c *websocket.Conn, v any) error {
	_ = c.SetWriteDeadline(time.Now().Add(writeWait))
	return c.WriteJSON(v)
}

func writeJSON(c *websocket.Conn, mu *sync.Mutex, v any) error {
	mu.Lock()
	defer mu.Unlock()
	return writeConn(c, v)
}
