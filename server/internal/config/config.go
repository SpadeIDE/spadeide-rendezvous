package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default session caps — finite on purpose. Operators who need more set env
// explicitly; 0 for bytes still means unlimited (documented escape hatch).
const (
	DefaultSessionMaxBytes = int64(512 << 20) // 512 MiB
	DefaultSessionMaxSecs  = 8 * 3600         // 8 hours
)

// Config is loaded only from the environment (no secrets on disk except ACME).
type Config struct {
	Domain          string
	Listen          string
	UDPListen       string
	ACMECache       string
	ACMEEmail       string
	MetricsListen   string
	AgentTTL        time.Duration
	SessionMaxBytes int64
	SessionMaxSecs  int
	MaxAgentsPerIP  int
	LookupsPerMin   int
	LogLevel        string
	DevInsecureTLS  bool // self-signed for local smoke (no ACME)
	Version         string
}

func FromEnv() (*Config, error) {
	agentTTL, err := durationEnv("SPADE_RV_AGENT_TTL", 300*time.Second)
	if err != nil {
		return nil, err
	}
	sessionBytes, err := int64Env("SPADE_RV_SESSION_MAX_BYTES", DefaultSessionMaxBytes)
	if err != nil {
		return nil, err
	}
	sessionSecs, err := intEnv("SPADE_RV_SESSION_MAX_SECONDS", DefaultSessionMaxSecs)
	if err != nil {
		return nil, err
	}
	maxAgents, err := intEnv("SPADE_RV_MAX_AGENTS_PER_IP", 20)
	if err != nil {
		return nil, err
	}
	lookups, err := intEnv("SPADE_RV_RATE_LOOKUPS_PER_MIN", 60)
	if err != nil {
		return nil, err
	}

	c := &Config{
		Domain:          os.Getenv("SPADE_RV_DOMAIN"),
		Listen:          envOr("SPADE_RV_LISTEN", ":443"),
		UDPListen:       envOr("SPADE_RV_UDP_LISTEN", ":443"),
		ACMECache:       envOr("SPADE_RV_ACME_CACHE", "/var/lib/spade-rendezvous/acme"),
		ACMEEmail:       os.Getenv("SPADE_RV_ACME_EMAIL"),
		MetricsListen:   envOr("SPADE_RV_METRICS_LISTEN", "127.0.0.1:9090"),
		AgentTTL:        agentTTL,
		SessionMaxBytes: sessionBytes,
		SessionMaxSecs:  sessionSecs,
		MaxAgentsPerIP:  maxAgents,
		LookupsPerMin:   lookups,
		LogLevel:        envOr("SPADE_RV_LOG_LEVEL", "info"),
		DevInsecureTLS:  os.Getenv("SPADE_RV_DEV_INSECURE") == "1",
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) Validate() error {
	if c.Domain == "" && !c.DevInsecureTLS {
		return fmt.Errorf("SPADE_RV_DOMAIN is required (or set SPADE_RV_DEV_INSECURE=1 for local smoke)")
	}
	if c.SessionMaxBytes < 0 {
		return fmt.Errorf("SPADE_RV_SESSION_MAX_BYTES must be >= 0 (0 = unlimited)")
	}
	if c.SessionMaxSecs <= 0 {
		return fmt.Errorf("SPADE_RV_SESSION_MAX_SECONDS must be > 0")
	}
	if c.AgentTTL <= 0 {
		return fmt.Errorf("SPADE_RV_AGENT_TTL must be > 0")
	}
	if c.MaxAgentsPerIP < 0 {
		return fmt.Errorf("SPADE_RV_MAX_AGENTS_PER_IP must be >= 0")
	}
	if c.LookupsPerMin < 0 {
		return fmt.Errorf("SPADE_RV_RATE_LOOKUPS_PER_MIN must be >= 0")
	}
	if err := assertLoopbackMetrics(c.MetricsListen); err != nil {
		return err
	}
	return nil
}

func assertLoopbackMetrics(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("SPADE_RV_METRICS_LISTEN: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("SPADE_RV_METRICS_LISTEN must be loopback (got %q) — refusing to start", addr)
	}
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func intEnv(k string, def int) (int, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", k, v)
	}
	return n, nil
}

func int64Env(k string, def int64) (int64, error) {
	v := os.Getenv(k)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", k, v)
	}
	return n, nil
}

func durationEnv(k string, def time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q", k, v)
	}
	return d, nil
}
