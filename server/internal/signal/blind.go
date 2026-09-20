package signal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"strings"
)

const (
	ridDomain   = "spade-rv"
	proveDomain = "spade-prove"
	ridBytes    = 16
)

// RID mirrors agent/internal/rendezvous — kept here for server-side tests.
func RID(secret []byte, epoch uint64) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(ridDomain))
	var eb [8]byte
	binary.BigEndian.PutUint64(eb[:], epoch)
	_, _ = mac.Write(eb[:])
	sum := mac.Sum(nil)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:ridBytes]))
}

func ProveMAC(secret, nonce []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(proveDomain))
	_, _ = mac.Write(nonce)
	return mac.Sum(nil)
}

func VerifyProveMAC(secret, nonce, got []byte) bool {
	return hmac.Equal(ProveMAC(secret, nonce), got)
}
