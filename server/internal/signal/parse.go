package signal

import (
	"encoding/json"
	"fmt"

	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

// ParseControlMessage unmarshals one introducer JSON frame. Rejects oversized
// payloads even when the caller forgot SetReadLimit (defense in depth).
func ParseControlMessage(raw []byte) (envelope, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty")
	}
	if int64(len(raw)) > wsutil.MaxControlMessage {
		return nil, fmt.Errorf("too large")
	}
	var msg envelope
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("null")
	}
	return msg, nil
}
