package notice

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetMissingFile(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "missing.json"))
	if s.Get() != nil {
		t.Fatal("expected nil for missing file")
	}
}

func TestDisabledAndHotReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notice.json")
	s := New(path)

	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
		_ = os.Chtimes(path, time.Now(), time.Now())
		s.Refresh()
	}

	write(`{"enabled":false,"id":"a","title":"Hello","body":"World"}`)
	if s.Get() != nil {
		t.Fatal("disabled notice must not publish")
	}

	write(`{"enabled":true,"id":"a","title":"Hello","body":"World","severity":"warning"}`)
	n := s.Get()
	if n == nil || n.ID != "a" || n.Severity != "warning" {
		t.Fatalf("got %#v", n)
	}

	write(`{"enabled":true,"id":"b","title":"Next","body":"Up"}`)
	n = s.Get()
	if n == nil || n.ID != "b" || n.Severity != "info" {
		t.Fatalf("reload got %#v", n)
	}

	write(`{"enabled":true}`)
	if s.Get() != nil {
		t.Fatal("empty id should clear notice")
	}
}

func TestDefaultPath(t *testing.T) {
	s := New("")
	if s.path != DefaultPath {
		t.Fatalf("path %q", s.path)
	}
}

func TestAttach(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notice.json")
	if err := os.WriteFile(path, []byte(`{"enabled":true,"id":"x","title":"T","body":"B"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	env := map[string]any{"t": "absent"}
	s.Attach(env)
	raw, ok := env["notice"]
	if !ok {
		t.Fatal("notice not attached")
	}
	n, ok := raw.(*Notice)
	if !ok || n.ID != "x" {
		t.Fatalf("attached %#v", raw)
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"enabled"`) {
		t.Fatalf("enabled leaked: %s", b)
	}
}

func TestHTTPHandler(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notice.json")
	s := New(path)

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/notice", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("empty status %d", rr.Code)
	}

	if err := os.WriteFile(path, []byte(`{"enabled":true,"id":"z","title":"Hi","body":"There"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	_ = os.Chtimes(path, time.Now(), time.Now())
	s.Refresh()

	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/notice", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var n Notice
	if err := json.NewDecoder(rr.Body).Decode(&n); err != nil {
		t.Fatal(err)
	}
	if n.ID != "z" {
		t.Fatalf("got %#v", n)
	}
}

func TestRejectNonHTTPSAndOversized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notice.json")
	s := New(path)

	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
		_ = os.Chtimes(path, time.Now(), time.Now())
		s.Refresh()
	}

	write(`{"enabled":true,"id":"a","title":"T","body":"B","agentUrl":"http://evil.example/a"}`)
	if s.Get() != nil {
		t.Fatal("http URL must be rejected")
	}

	write(`{"enabled":true,"id":"a","title":"T","body":"B","agentUrl":"https://spadeide.com/a","appUrl":"https://spadeide.com/b"}`)
	if s.Get() == nil {
		t.Fatal("https URLs must be accepted")
	}

	big := `{"enabled":true,"id":"big","title":"T","body":"` + strings.Repeat("x", MaxFileBytes) + `"}`
	write(big)
	if s.Get() != nil {
		t.Fatal("oversized notice must be rejected")
	}
}

func TestGetDoesNotStat(t *testing.T) {
	// A10: Get returns the cached value; Refresh is what picks up writes.
	path := filepath.Join(t.TempDir(), "notice.json")
	if err := os.WriteFile(path, []byte(`{"enabled":true,"id":"a","title":"T","body":"B"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if s.Get() == nil || s.Get().ID != "a" {
		t.Fatal("initial load")
	}
	if err := os.WriteFile(path, []byte(`{"enabled":true,"id":"b","title":"T","body":"B"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	_ = os.Chtimes(path, time.Now(), time.Now())
	if s.Get().ID != "a" {
		t.Fatal("Get must not hot-reload without Refresh")
	}
	s.Refresh()
	if s.Get().ID != "b" {
		t.Fatal("Refresh must pick up new id")
	}
}
