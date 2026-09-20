// Package wsutil holds shared WebSocket policy for the public rendezvous.
package wsutil

import (
	"net/http"

	"github.com/gorilla/websocket"
)

const (
	// MaxControlMessage caps JSON frames on /v1/agent and /v1/client
	// (hello / prove / lookup / offer-ack). Set before any ReadMessage.
	MaxControlMessage int64 = 64 << 10 // 64 KiB

	// MaxRelayMessage caps one blind-relay frame. Nested TLS rarely needs more;
	// without a cap a single guest can force a multi‑MiB allocation.
	MaxRelayMessage int64 = 1 << 20 // 1 MiB
)

// Upgrader rejects browser Origins. Native SpadeIDE clients omit Origin; a
// malicious page must not enlist visitors' browsers to flood the introducer.
func Upgrader() websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return r.Header.Get("Origin") == ""
		},
	}
}
