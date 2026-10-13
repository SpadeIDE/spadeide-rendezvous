package registry

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestMultiKeySameConn(t *testing.T) {
	r := New(time.Minute)
	conn := &websocket.Conn{}
	s := &Slot{
		Keys:     []string{"aid1", "rid0", "rid1"},
		Conn:     conn,
		RemoteIP: "1.2.3.4",
	}
	if !r.Upsert(s) {
		t.Fatal("upsert refused")
	}
	if r.Len() != 1 {
		t.Fatalf("len=%d", r.Len())
	}
	if r.CountByIP("1.2.3.4") != 1 {
		t.Fatalf("count by ip")
	}
	for _, k := range s.Keys {
		got, ok := r.Get(k)
		if !ok || got != s {
			t.Fatalf("get %s", k)
		}
	}
	r.Remove("rid0", conn)
	if r.Len() != 0 {
		t.Fatalf("after remove len=%d", r.Len())
	}
}

func TestUpsertRefusesTakeover(t *testing.T) {
	r := New(time.Minute)
	a := &websocket.Conn{}
	b := &websocket.Conn{}
	first := &Slot{Keys: []string{"rid-x", "aid-x"}, Conn: a, RemoteIP: "1.1.1.1"}
	if !r.Upsert(first) {
		t.Fatal("first upsert")
	}
	thief := &Slot{Keys: []string{"rid-x"}, Conn: b, RemoteIP: "9.9.9.9"}
	if r.Upsert(thief) {
		t.Fatal("takeover should be refused")
	}
	got, ok := r.Get("rid-x")
	if !ok || got != first || got.Conn != a {
		t.Fatalf("incumbent lost: ok=%v got=%v", ok, got)
	}
	// Same conn may refresh keys.
	refresh := &Slot{Keys: []string{"rid-x", "rid-y"}, Conn: a, RemoteIP: "1.1.1.1"}
	if !r.Upsert(refresh) {
		t.Fatal("same-conn refresh refused")
	}
	if _, ok := r.Get("rid-y"); !ok {
		t.Fatal("new key missing after refresh")
	}
}

func TestUpsertReplacesExpired(t *testing.T) {
	r := New(time.Millisecond)
	a := &websocket.Conn{}
	b := &websocket.Conn{}
	old := &Slot{Keys: []string{"rid-e"}, Conn: a, RemoteIP: "1.1.1.1"}
	if !r.Upsert(old) {
		t.Fatal("upsert")
	}
	time.Sleep(5 * time.Millisecond)
	next := &Slot{Keys: []string{"rid-e"}, Conn: b, RemoteIP: "2.2.2.2"}
	if !r.Upsert(next) {
		t.Fatal("expired slot should yield")
	}
	got, ok := r.Get("rid-e")
	if !ok || got.Conn != b {
		t.Fatalf("want new conn, ok=%v", ok)
	}
}
