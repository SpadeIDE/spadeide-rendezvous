package reflex

import (
	"log"
	"net"
	"strings"
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
func ListenAndServe(packetConn net.PacketConn) error {
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
		reply := MagicPong + " " + host + " " + port + "\n"
		if _, err := packetConn.WriteTo([]byte(reply), addr); err != nil {
			log.Printf("reflex: write: %v", err)
		}
	}
}
