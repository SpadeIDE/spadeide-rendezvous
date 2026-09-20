package ticket

import (
	"testing"
	"time"
)

func TestIssueVerifyHappy(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := m.Issue("sid-1", "client", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	got, err := m.Verify(enc)
	if err != nil {
		t.Fatal(err)
	}
	if got.SID != "sid-1" || got.Role != "client" {
		t.Fatalf("%+v", got)
	}
	if _, err := m.Verify(enc); err == nil {
		t.Fatal("expected reuse rejection")
	}
}
