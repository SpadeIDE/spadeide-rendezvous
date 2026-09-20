package config

import (
	"os"
	"testing"
	"time"
)

func TestBadIntExitsWithError(t *testing.T) {
	t.Setenv("SPADE_RV_DEV_INSECURE", "1")
	t.Setenv("SPADE_RV_SESSION_MAX_SECONDS", "not-a-number")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for bad SESSION_MAX_SECONDS")
	}
}

func TestBadDurationExitsWithError(t *testing.T) {
	t.Setenv("SPADE_RV_DEV_INSECURE", "1")
	t.Setenv("SPADE_RV_AGENT_TTL", "forever")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for bad AGENT_TTL")
	}
}

func TestDefaultsAreFinite(t *testing.T) {
	t.Setenv("SPADE_RV_DEV_INSECURE", "1")
	for _, k := range []string{
		"SPADE_RV_SESSION_MAX_BYTES",
		"SPADE_RV_SESSION_MAX_SECONDS",
		"SPADE_RV_AGENT_TTL",
		"SPADE_RV_MAX_AGENTS_PER_IP",
		"SPADE_RV_RATE_LOOKUPS_PER_MIN",
	} {
		_ = os.Unsetenv(k)
	}
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.SessionMaxBytes != DefaultSessionMaxBytes {
		t.Fatalf("SessionMaxBytes=%d want %d", c.SessionMaxBytes, DefaultSessionMaxBytes)
	}
	if c.SessionMaxSecs != DefaultSessionMaxSecs {
		t.Fatalf("SessionMaxSecs=%d want %d", c.SessionMaxSecs, DefaultSessionMaxSecs)
	}
	if c.AgentTTL != 300*time.Second {
		t.Fatalf("AgentTTL=%s", c.AgentTTL)
	}
}

func TestMetricsMustBeLoopback(t *testing.T) {
	t.Setenv("SPADE_RV_DEV_INSECURE", "1")
	t.Setenv("SPADE_RV_METRICS_LISTEN", "0.0.0.0:9090")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected rejection of public metrics bind")
	}
}
