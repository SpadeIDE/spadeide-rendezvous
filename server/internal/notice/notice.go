// Package notice loads an operator announcement from a JSON file and
// hot-reloads it when the file mtime changes (checked on Refresh / sweep).
package notice

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultPath is where install.sh places notice.json.
const DefaultPath = "/etc/spade-rendezvous/notice.json"

// MaxFileBytes caps the notice file so a huge document cannot be copied
// into every lookup response (A10).
const MaxFileBytes = 16 << 10 // 16 KiB

// Notice is the public payload attached to lookup responses and GET /v1/notice.
// File-local `enabled` is never sent to clients.
type Notice struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	Severity          string `json:"severity,omitempty"`
	BelowAppVersion   string `json:"belowAppVersion,omitempty"`
	BelowAgentVersion string `json:"belowAgentVersion,omitempty"`
	AgentURL          string `json:"agentUrl,omitempty"` // host agent download page
	AppURL            string `json:"appUrl,omitempty"`   // iPad / App Store link
}

// fileDoc is the on-disk shape. enabled=false → no announcement (file still present).
type fileDoc struct {
	Enabled           bool   `json:"enabled"`
	ID                string `json:"id"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	Severity          string `json:"severity"`
	BelowAppVersion   string `json:"belowAppVersion"`
	BelowAgentVersion string `json:"belowAgentVersion"`
	AgentURL          string `json:"agentUrl"`
	AppURL            string `json:"appUrl"`
	// Legacy single link — mapped to both URLs when the new fields are empty.
	URL string `json:"url"`
}

// Store watches a single JSON file.
type Store struct {
	path string

	mu    sync.Mutex
	mtime time.Time
	cur   *Notice
}

func New(path string) *Store {
	path = strings.TrimSpace(path)
	if path == "" {
		path = DefaultPath
	}
	s := &Store{path: path}
	s.Refresh()
	return s
}

// Refresh re-reads the file when mtime changes. Call from the server sweep
// ticker (A10) — Get itself does not Stat.
func (s *Store) Refresh() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshLocked()
}

// Get returns the current notice when enabled, or nil.
func (s *Store) Get() *Notice {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return nil
	}
	cp := *s.cur
	return &cp
}

// Attach copies the notice into an envelope map under key "notice".
func (s *Store) Attach(env map[string]any) {
	n := s.Get()
	if n == nil {
		return
	}
	env["notice"] = n
}

// Handler serves GET /v1/notice as JSON (200 + body, or 204 when disabled/empty).
func (s *Store) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		n := s.Get()
		if n == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_ = json.NewEncoder(w).Encode(n)
	}
}

func (s *Store) refreshLocked() {
	info, err := os.Stat(s.path)
	if err != nil {
		s.cur = nil
		s.mtime = time.Time{}
		return
	}
	mt := info.ModTime()
	if !s.mtime.IsZero() && mt.Equal(s.mtime) {
		return
	}
	s.mtime = mt
	if info.Size() > MaxFileBytes {
		s.cur = nil
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		s.cur = nil
		return
	}
	if len(data) > MaxFileBytes {
		s.cur = nil
		return
	}
	data = trimBOM(data)
	if len(strings.TrimSpace(string(data))) == 0 {
		s.cur = nil
		return
	}
	var doc fileDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		s.cur = nil
		return
	}
	if !doc.Enabled {
		s.cur = nil
		return
	}
	agentURL := strings.TrimSpace(doc.AgentURL)
	appURL := strings.TrimSpace(doc.AppURL)
	if legacy := strings.TrimSpace(doc.URL); legacy != "" {
		if agentURL == "" {
			agentURL = legacy
		}
		if appURL == "" {
			appURL = legacy
		}
	}
	if !httpsURLOK(agentURL) || !httpsURLOK(appURL) {
		s.cur = nil
		return
	}
	n := Notice{
		ID:                strings.TrimSpace(doc.ID),
		Title:             strings.TrimSpace(doc.Title),
		Body:              strings.TrimSpace(doc.Body),
		Severity:          strings.ToLower(strings.TrimSpace(doc.Severity)),
		BelowAppVersion:   strings.TrimSpace(doc.BelowAppVersion),
		BelowAgentVersion: strings.TrimSpace(doc.BelowAgentVersion),
		AgentURL:          agentURL,
		AppURL:            appURL,
	}
	if n.ID == "" || (n.Title == "" && n.Body == "") {
		s.cur = nil
		return
	}
	switch n.Severity {
	case "", "info", "warning", "critical":
		if n.Severity == "" {
			n.Severity = "info"
		}
	default:
		n.Severity = "info"
	}
	s.cur = &n
}

// httpsURLOK accepts empty (optional) or https:// URLs only.
func httpsURLOK(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "https" || u.Host == "" {
		return false
	}
	return true
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
