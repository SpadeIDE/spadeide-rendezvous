package ticket

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// Ticket is a short-lived, single-use relay admission.
type Ticket struct {
	SID  string `json:"sid"`
	Exp  int64  `json:"exp"`
	Role string `json:"role"` // "agent" | "client"
}

type Manager struct {
	mu      sync.Mutex
	pub     ed25519.PublicKey
	priv    ed25519.PrivateKey
	rotated time.Time
	used    map[string]struct{}
}

func New() (*Manager, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Manager{
		pub:     pub,
		priv:    priv,
		rotated: time.Now(),
		used:    make(map[string]struct{}),
	}, nil
}

func (m *Manager) maybeRotate() {
	if time.Since(m.rotated) < time.Hour {
		return
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return
	}
	m.pub, m.priv = pub, priv
	m.rotated = time.Now()
	m.used = make(map[string]struct{})
}

func (m *Manager) Issue(sid, role string, ttl time.Duration) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maybeRotate()
	t := Ticket{SID: sid, Exp: time.Now().Add(ttl).Unix(), Role: role}
	raw, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(m.priv, raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (m *Manager) Verify(encoded string) (*Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	parts := splitDot(encoded)
	if len(parts) != 2 {
		return nil, errors.New("bad ticket")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(m.pub, raw, sig) {
		return nil, errors.New("bad signature")
	}
	var t Ticket
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	if time.Now().Unix() > t.Exp {
		return nil, errors.New("expired")
	}
	if _, ok := m.used[encoded]; ok {
		return nil, errors.New("reused")
	}
	m.used[encoded] = struct{}{}
	return &t, nil
}

func splitDot(s string) []string {
	i := -1
	for j := 0; j < len(s); j++ {
		if s[j] == '.' {
			i = j
			break
		}
	}
	if i < 0 {
		return nil
	}
	return []string{s[:i], s[i+1:]}
}
