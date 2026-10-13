package signal

import (
	"encoding/json"
	"fmt"

	"github.com/spadeide/spade-rendezvous/internal/notice"
	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

// Typed introducer frames (B5). Unknown JSON fields are ignored so older
// clients keep working. Wire names stay the string literals below.

type HelloMsg struct {
	T     string   `json:"t"`
	AID   string   `json:"aid,omitempty"`
	RID   string   `json:"rid,omitempty"`
	RIDs  []string `json:"rids,omitempty"`
	Proto int      `json:"proto,omitempty"`
}

type ProveMsg struct {
	T   string `json:"t"`
	Pub string `json:"pub,omitempty"`
	Sig string `json:"sig,omitempty"`
	MAC string `json:"mac,omitempty"`
}

type LookupMsg struct {
	T    string `json:"t"`
	AID  string `json:"aid,omitempty"`
	RID  string `json:"rid,omitempty"`
	Mode string `json:"mode,omitempty"`
}

type OfferAckMsg struct {
	T      string `json:"t"`
	SID    string `json:"sid,omitempty"`
	Direct string `json:"direct,omitempty"`
}

type ChallengeMsg struct {
	T     string `json:"t"`
	Nonce string `json:"nonce"`
}

type ReadyMsg struct {
	T   string `json:"t"`
	TTL int    `json:"ttl"`
}

type ErrorMsg struct {
	T   string `json:"t"`
	Msg string `json:"msg"`
}

type OfferMsg struct {
	T      string `json:"t"`
	SID    string `json:"sid"`
	Ticket string `json:"ticket"`
	Mode   string `json:"mode"`
}

type PeerMsg struct {
	T      string         `json:"t"`
	SID    string         `json:"sid"`
	Ticket string         `json:"ticket"`
	Direct string         `json:"direct,omitempty"`
	Notice *notice.Notice `json:"notice,omitempty"`
}

type AbsentMsg struct {
	T      string         `json:"t"`
	Notice *notice.Notice `json:"notice,omitempty"`
}

type DirectMsg struct {
	T      string `json:"t"`
	Direct string `json:"direct"`
}

type PongMsg struct {
	T string `json:"t"`
}

func checkControlRaw(raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("empty")
	}
	if int64(len(raw)) > wsutil.MaxControlMessage {
		return fmt.Errorf("too large")
	}
	return nil
}

// DecodeControl unmarshals raw into dst after size checks.
func DecodeControl(raw []byte, dst any) error {
	if err := checkControlRaw(raw); err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return err
	}
	return nil
}

// PeekType returns the "t" field without fully decoding the body.
func PeekType(raw []byte) (string, error) {
	var head struct {
		T string `json:"t"`
	}
	if err := DecodeControl(raw, &head); err != nil {
		return "", err
	}
	return head.T, nil
}
