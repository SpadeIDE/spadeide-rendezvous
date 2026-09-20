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
	r.Upsert(s)
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
