package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/spadeide/spade-rendezvous/internal/config"
)

func testCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Domain:          "localhost",
		DevInsecureTLS:  true,
		Listen:          ":0",
		UDPListen:       "",
		MetricsListen:   "127.0.0.1:0",
		AgentTTL:        5 * time.Minute,
		SessionMaxBytes: 1 << 20,
		SessionMaxSecs:  60,
		MaxAgentsPerIP:  20,
		LookupsPerMin:   1000,
		AgentPerMin:     1000,
		ReflexPerMin:    1000,
		NoticeFile:      filepath.Join(t.TempDir(), "notice.json"),
	}
}

func TestRelayValidTicketPairs(t *testing.T) {
	w, err := NewWire(testCfg(t))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(w.Mux)
	t.Cleanup(srv.Close)
	base := "ws" + strings.TrimPrefix(srv.URL, "http")

	sid := "sess-1"
	agentTkt, err := w.Tickets.Issue(sid, "agent", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	clientTkt, err := w.Tickets.Issue(sid, "client", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	agentConn, resp, err := websocket.DefaultDialer.Dial(base+"/v1/relay?ticket="+agentTkt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer func() { _ = agentConn.Close() }()

	clientConn, resp, err := websocket.DefaultDialer.Dial(base+"/v1/relay?ticket="+clientTkt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer func() { _ = clientConn.Close() }()

	payload := []byte("hello-relay")
	if err := agentConn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, got, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("got %q", got)
	}
}

func TestRelayBadTicket401(t *testing.T) {
	w, err := NewWire(testCfg(t))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(w.Mux)
	t.Cleanup(srv.Close)

	_, resp, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/relay?ticket=bogus", nil)
	if err == nil {
		t.Fatal("expected dial failure for bad ticket")
	}
	if resp == nil {
		t.Fatal("expected HTTP response")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}

func TestRelayBanned429(t *testing.T) {
	w, err := NewWire(testCfg(t))
	if err != nil {
		t.Fatal(err)
	}
	// Ban a key that BanKey will produce for the httptest remote addr (127.0.0.1).
	w.Bans.Strike("127.0.0.1")

	srv := httptest.NewServer(w.Mux)
	t.Cleanup(srv.Close)

	tkt, err := w.Tickets.Issue("s", "agent", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, resp, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/relay?ticket="+tkt, nil)
	if err == nil {
		t.Fatal("expected dial failure while banned")
	}
	if resp == nil {
		t.Fatal("expected HTTP response")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}
