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

func TestTicketSurvivesRotation(t *testing.T) {
	// A8: ticket issued just before rotation must still verify.
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := m.Issue("sid-rot", "agent", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.rotated = time.Now().Add(-2 * time.Hour)
	m.mu.Unlock()
	m.Sweep(time.Now())
	got, err := m.Verify(enc)
	if err != nil {
		t.Fatalf("pre-rotation ticket rejected after Sweep rotate: %v", err)
	}
	if got.SID != "sid-rot" {
		t.Fatalf("%+v", got)
	}
}

func TestPrevKeyExpires(t *testing.T) {
	m, err := New()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := m.Issue("sid-old", "client", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.rotated = time.Now().Add(-2 * time.Hour)
	m.mu.Unlock()
	now := time.Now()
	m.Sweep(now)
	// Force prev key past grace.
	m.mu.Lock()
	m.prevUntil = now.Add(-time.Second)
	m.mu.Unlock()
	m.Sweep(now)
	if _, err := m.Verify(enc); err == nil {
		t.Fatal("expected bad signature after prev key dropped")
	}
}
