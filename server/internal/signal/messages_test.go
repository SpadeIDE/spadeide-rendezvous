package signal

import (
	"testing"

	"github.com/spadeide/spade-rendezvous/internal/wsutil"
)

func TestDecodeControlHello(t *testing.T) {
	raw := []byte(`{"t":"hello","aid":"ABC","rids":["r1","r2"],"proto":2,"extra":true}`)
	var hello HelloMsg
	if err := DecodeControl(raw, &hello); err != nil {
		t.Fatal(err)
	}
	if hello.T != "hello" || hello.AID != "ABC" || hello.Proto != 2 {
		t.Fatalf("%+v", hello)
	}
	if len(hello.RIDs) != 2 || hello.RIDs[0] != "r1" {
		t.Fatalf("rids %+v", hello.RIDs)
	}
}

func TestDecodeControlRejectsHuge(t *testing.T) {
	raw := make([]byte, int(wsutil.MaxControlMessage)+1)
	var hello HelloMsg
	if err := DecodeControl(raw, &hello); err == nil {
		t.Fatal("expected too large")
	}
}

func TestPeekType(t *testing.T) {
	got, err := PeekType([]byte(`{"t":"offer-ack","sid":"x"}`))
	if err != nil || got != "offer-ack" {
		t.Fatalf("got %q err=%v", got, err)
	}
}
