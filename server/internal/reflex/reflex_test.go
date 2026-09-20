package reflex

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestEchoObservedAddr(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	go func() { _ = ListenAndServe(pc) }()

	client, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	rvAddr := pc.LocalAddr().(*net.UDPAddr)
	if _, err := client.WriteTo([]byte(MagicPing+"\n"), rvAddr); err != nil {
		t.Fatal(err)
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, _, err := client.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	msg := strings.TrimSpace(string(buf[:n]))
	if !strings.HasPrefix(msg, MagicPong) {
		t.Fatalf("got %q", msg)
	}
	parts := strings.Fields(msg)
	if len(parts) < 3 {
		t.Fatalf("short reply %q", msg)
	}
	wantHost, wantPort, _ := net.SplitHostPort(client.LocalAddr().String())
	if parts[1] != wantHost || parts[2] != wantPort {
		t.Fatalf("got %s:%s want %s:%s", parts[1], parts[2], wantHost, wantPort)
	}
}
