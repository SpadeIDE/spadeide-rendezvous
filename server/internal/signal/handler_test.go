package signal

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/spadeide/spade-rendezvous/internal/ratelimit"
	"github.com/spadeide/spade-rendezvous/internal/registry"
	"github.com/spadeide/spade-rendezvous/internal/ticket"
)

func testHandler(t *testing.T) (*Handler, *httptest.Server) {
	t.Helper()
	tickets, err := ticket.New()
	if err != nil {
		t.Fatal(err)
	}
	bans := ratelimit.NewBanbook()
	h := &Handler{
		Reg:     registry.New(5 * time.Minute),
		Tickets: tickets,
		TTL:     5 * time.Minute,
		Bans:    bans,
		Agents: &ratelimit.Limiter{
			Bans: bans,
			Soft: ratelimit.NewWindow(1000, time.Minute),
		},
		Lookups: &ratelimit.Limiter{
			Bans: bans,
			Soft: ratelimit.NewWindow(1000, time.Minute),
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/agent", h.Agent)
	mux.HandleFunc("/v1/client", h.Client)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return h, srv
}

func dialWS(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	c, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func wsURL(srv *httptest.Server, path string) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + path
}

func readEnv(t *testing.T, c *websocket.Conn) envelope {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, raw, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseControlMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func registerAID(t *testing.T, srv *httptest.Server, pub ed25519.PublicKey, priv ed25519.PrivateKey) *websocket.Conn {
	t.Helper()
	aid := DeriveAID(pub)
	c := dialWS(t, wsURL(srv, "/v1/agent"))
	if err := c.WriteJSON(map[string]any{"t": "hello", "aid": aid, "proto": 2}); err != nil {
		t.Fatal(err)
	}
	ch := readEnv(t, c)
	nonce, err := base64.StdEncoding.DecodeString(ch["nonce"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteJSON(map[string]any{
		"t":   "prove",
		"pub": base64.StdEncoding.EncodeToString(pub),
		"sig": base64.StdEncoding.EncodeToString(ed25519.Sign(priv, nonce)),
	}); err != nil {
		t.Fatal(err)
	}
	if readEnv(t, c)["t"] != "ready" {
		t.Fatal("expected ready")
	}
	return c
}

func TestAgentHandshakeEd25519(t *testing.T) {
	h, srv := testHandler(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = registerAID(t, srv, pub, priv)
	if _, ok := h.Reg.Get(DeriveAID(pub)); !ok {
		t.Fatal("aid not registered")
	}
}

func registerAIDWithRIDs(t *testing.T, srv *httptest.Server, pub ed25519.PublicKey, priv ed25519.PrivateKey, secret []byte, rids []string) *websocket.Conn {
	t.Helper()
	aid := DeriveAID(pub)
	c := dialWS(t, wsURL(srv, "/v1/agent"))
	hello := map[string]any{"t": "hello", "aid": aid, "proto": 2}
	if len(rids) > 0 {
		hello["rids"] = rids
	}
	if err := c.WriteJSON(hello); err != nil {
		t.Fatal(err)
	}
	ch := readEnv(t, c)
	nonce, err := base64.StdEncoding.DecodeString(ch["nonce"].(string))
	if err != nil {
		t.Fatal(err)
	}
	prove := map[string]any{
		"t":   "prove",
		"pub": base64.StdEncoding.EncodeToString(pub),
		"sig": base64.StdEncoding.EncodeToString(ed25519.Sign(priv, nonce)),
	}
	if len(secret) > 0 {
		prove["mac"] = base64.StdEncoding.EncodeToString(ProveMAC(secret, nonce))
	}
	if err := c.WriteJSON(prove); err != nil {
		t.Fatal(err)
	}
	if readEnv(t, c)["t"] != "ready" {
		t.Fatal("expected ready")
	}
	return c
}

func TestA1SlotTakeoverRefused(t *testing.T) {
	h, srv := testHandler(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	rid := RID(secret, 1)
	aid := DeriveAID(pub)

	first := registerAIDWithRIDs(t, srv, pub, priv, secret, []string{rid})

	// Second connection with same aid+rid but different socket → slot taken.
	second := dialWS(t, wsURL(srv, "/v1/agent"))
	_ = second.WriteJSON(map[string]any{"t": "hello", "aid": aid, "rids": []string{rid}, "proto": 2})
	ch := readEnv(t, second)
	nonce, _ := base64.StdEncoding.DecodeString(ch["nonce"].(string))
	_ = second.WriteJSON(map[string]any{
		"t":   "prove",
		"pub": base64.StdEncoding.EncodeToString(pub),
		"sig": base64.StdEncoding.EncodeToString(ed25519.Sign(priv, nonce)),
		"mac": base64.StdEncoding.EncodeToString(ProveMAC(secret, nonce)),
	})
	errEnv := readEnv(t, second)
	if errEnv["t"] != "error" || errEnv["msg"] != "slot taken" {
		t.Fatalf("want slot taken, got %#v", errEnv)
	}
	if _, ok := h.Reg.Get(rid); !ok {
		t.Fatal("incumbent must keep the rid")
	}
	if _, ok := h.Reg.Get(aid); !ok {
		t.Fatal("incumbent must keep the aid")
	}
	if err := first.WriteJSON(map[string]any{"t": "ping"}); err != nil {
		t.Fatal(err)
	}
	if readEnv(t, first)["t"] != "pong" {
		t.Fatal("incumbent session dead")
	}
}

func TestRidOnlyGhostRejected(t *testing.T) {
	// Unpaired iPad holds rvSecret but not the agent Ed25519 key — must not
	// be able to register a ghost agent under that rid alone.
	_, srv := testHandler(t)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	rid := RID(secret, 1)

	c := dialWS(t, wsURL(srv, "/v1/agent"))
	_ = c.WriteJSON(map[string]any{"t": "hello", "rids": []string{rid}, "proto": 2})
	errEnv := readEnv(t, c)
	if errEnv["t"] != "error" || errEnv["msg"] != "aid required" {
		t.Fatalf("want aid required, got %#v", errEnv)
	}
}

func TestLookupOfferPeer(t *testing.T) {
	_, srv := testHandler(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	aid := DeriveAID(pub)
	agent := registerAID(t, srv, pub, priv)

	offerCh := make(chan envelope, 1)
	go func() {
		_ = agent.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, raw, err := agent.ReadMessage()
		if err != nil {
			offerCh <- envelope{"t": "error", "msg": err.Error()}
			return
		}
		m, err := ParseControlMessage(raw)
		if err != nil {
			offerCh <- envelope{"t": "error", "msg": err.Error()}
			return
		}
		offerCh <- m
	}()

	client := dialWS(t, wsURL(srv, "/v1/client"))
	if err := client.WriteJSON(map[string]any{"t": "lookup", "aid": aid, "mode": "full"}); err != nil {
		t.Fatal(err)
	}

	offer := <-offerCh
	if offer["t"] != "offer" {
		t.Fatalf("agent offer %#v", offer)
	}
	sid, _ := offer["sid"].(string)
	if err := agent.WriteJSON(map[string]any{"t": "offer-ack", "sid": sid, "direct": ""}); err != nil {
		t.Fatal(err)
	}

	peer := readEnv(t, client)
	if peer["t"] != "peer" {
		t.Fatalf("client peer %#v", peer)
	}
	if peer["sid"] != sid {
		t.Fatalf("sid mismatch peer=%v offer=%v", peer["sid"], sid)
	}
	if _, ok := peer["ticket"].(string); !ok || peer["ticket"] == "" {
		t.Fatal("missing client ticket")
	}
}

func TestLookupAbsent(t *testing.T) {
	_, srv := testHandler(t)
	client := dialWS(t, wsURL(srv, "/v1/client"))
	_ = client.WriteJSON(map[string]any{"t": "lookup", "aid": "missingagent"})
	absent := readEnv(t, client)
	if absent["t"] != "absent" {
		t.Fatalf("got %#v", absent)
	}
}

func TestMalformedLookupNoBan(t *testing.T) {
	h, srv := testHandler(t)
	client := dialWS(t, wsURL(srv, "/v1/client"))
	_ = client.WriteJSON(map[string]any{"t": "not-lookup"})
	errEnv := readEnv(t, client)
	if errEnv["t"] != "error" {
		t.Fatalf("got %#v", errEnv)
	}
	if h.Bans.StrikesTotal() != 0 {
		t.Fatal("A11: first malformed lookup must not Strike")
	}
}
