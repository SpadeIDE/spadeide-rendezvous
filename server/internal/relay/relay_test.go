package relay

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Early writer must not lose its first application frame when the peer
// arrives a moment later (the nested-TLS ClientHello case).
func TestHubPreservesEarlyBytes(t *testing.T) {
	hub := New(0, time.Minute)
	hub.pairWait = 5 * time.Second

	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	var wg sync.WaitGroup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		sid := r.URL.Query().Get("sid")
		role := r.URL.Query().Get("role")
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.Arrive(sid, role, conn)
		}()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/?sid=test"

	clientA, _, err := websocket.DefaultDialer.Dial(wsURL+"&role=agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientA.Close()

	hello := []byte{0x16, 0x03, 0x03, 0x00, 0x05, 'H', 'E', 'L', 'L', 'O'}
	if err := clientA.WriteMessage(websocket.BinaryMessage, hello); err != nil {
		t.Fatal(err)
	}
	// Let the frame sit unread on the hub side (first arriver).
	time.Sleep(50 * time.Millisecond)

	clientB, _, err := websocket.DefaultDialer.Dial(wsURL+"&role=client", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientB.Close()

	_ = clientB.SetReadDeadline(time.Now().Add(2 * time.Second))
	mt, data, err := clientB.ReadMessage()
	if err != nil {
		t.Fatalf("peer did not receive early bytes: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("mt=%d", mt)
	}
	if string(data) != string(hello) {
		t.Fatalf("got %q want %q", data, hello)
	}

	clientA.Close()
	clientB.Close()
	wg.Wait()
}
