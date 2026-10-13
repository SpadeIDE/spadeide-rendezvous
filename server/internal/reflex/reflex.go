package reflex

import (
	"log"
	"net"
	"strings"

	"github.com/spadeide/spade-rendezvous/internal/ratelimit"
)

const (
	MagicPing = "SPADE-REFLEX"
	MagicPong = "SPADE-REFLEX-OK"
	maxPacket = 256
)

// ListenAndServe echoes the observed source address so agents can learn their
// reflexive UDP mapping (hole-punch candidate). Protocol:
//
//	→ "SPADE-REFLEX\n"
//	← "SPADE-REFLEX-OK <ip> <port>\n"
//
// limit may be nil (tests). When set, use a Soft-only Limiter (Bans == nil):
// over-limit sources are dropped silently with no Strike (A2 — UDP source
// addresses are spoofable and must not feed the HTTP Banbook).
func ListenAndServe(packetConn net.PacketConn, limit *ratelimit.Limiter) error {
	buf := make([]byte, maxPacket)
	for {
		n, addr, err := packetConn.ReadFrom(buf)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		msg := strings.TrimSpace(string(buf[:n]))
		if !strings.HasPrefix(msg, MagicPing) {
			continue
		}
		host, port, err := net.SplitHostPort(addr.String())
		if err != nil {
			continue
		}
		if limit != nil {
			if banned, _ := limit.CheckBan(host); banned {
				continue // silent — do not amplify
			}
			if !limit.Allow(host) {
				continue
			}
		}
		reply := MagicPong + " " + host + " " + port + "\n"
		if _, err := packetConn.WriteTo([]byte(reply), addr); err != nil {
			log.Printf("reflex: write: %v", err)
		}
	}
}
