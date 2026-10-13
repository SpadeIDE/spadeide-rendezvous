package signal

import (
	"encoding/json"
	"fmt"
)

// ParseControlMessage unmarshals one introducer JSON frame into a generic
// envelope. Prefer DecodeControl into typed structs (messages.go) in handlers.
// Kept for fuzz and tolerant map-shaped inspection.
func ParseControlMessage(raw []byte) (envelope, error) {
	if err := checkControlRaw(raw); err != nil {
		return nil, err
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
