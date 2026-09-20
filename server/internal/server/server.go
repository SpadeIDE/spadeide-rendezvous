package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/spadeide/spade-rendezvous/internal/config"
	"github.com/spadeide/spade-rendezvous/internal/metrics"
	"github.com/spadeide/spade-rendezvous/internal/ratelimit"
	"github.com/spadeide/spade-rendezvous/internal/reflex"
	"github.com/spadeide/spade-rendezvous/internal/registry"
	"github.com/spadeide/spade-rendezvous/internal/relay"
	"github.com/spadeide/spade-rendezvous/internal/signal"
	"github.com/spadeide/spade-rendezvous/internal/ticket"
	"github.com/spadeide/spade-rendezvous/internal/wsutil"
	"golang.org/x/crypto/acme/autocert"
)

func ListenAndServe(ctx context.Context, cfg *config.Config) error {
	reg := registry.New(cfg.AgentTTL)
	tickets, err := ticket.New()
	if err != nil {
		return err
	}
	hub := relay.New(cfg.SessionMaxBytes, time.Duration(cfg.SessionMaxSecs)*time.Second)
	counters := &metrics.Counters{}
	hub.OnBytes(func(n int64) { counters.RelayBytes.Add(n) })
	sig := &signal.Handler{
		Reg:            reg,
		Tickets:        tickets,
		TTL:            cfg.AgentTTL,
		MaxAgentsPerIP: cfg.MaxAgentsPerIP,
		Lookups:        ratelimit.NewWindow(cfg.LookupsPerMin, time.Minute),
		Counters:       counters,
	}

	if cfg.UDPListen != "" {
		udp, err := net.ListenPacket("udp", cfg.UDPListen)
		if err != nil {
			return fmt.Errorf("udp listen %s: %w", cfg.UDPListen, err)
		}
		go func() {
			<-ctx.Done()
			_ = udp.Close()
		}()
		go func() {
			log.Printf("reflex UDP on %s", cfg.UDPListen)
			if err := reflex.ListenAndServe(udp); err != nil && ctx.Err() == nil {
				log.Printf("reflex: %v", err)
			}
		}()
	}

	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				reg.Sweep()
				counters.AgentsOnline.Store(int64(reg.Len()))
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/agent", sig.Agent)
	mux.HandleFunc("/v1/client", sig.Client)
	mux.HandleFunc("/v1/relay", func(w http.ResponseWriter, r *http.Request) {
		tkt := r.URL.Query().Get("ticket")
		parsed, err := tickets.Verify(tkt)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		up := wsutil.Upgrader()
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn.SetReadLimit(wsutil.MaxRelayMessage)
		counters.SessionsStart.Add(1)
		hub.Arrive(parsed.SID, parsed.Role, conn)
	})

	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/metrics", counters.Handler())
	go func() {
		ln, err := net.Listen("tcp", cfg.MetricsListen)
		if err != nil {
			log.Printf("metrics listen: %v", err)
			return
		}
		log.Printf("metrics on http://%s/metrics", cfg.MetricsListen)
		srv := &http.Server{Handler: metricsMux}
		go func() {
			<-ctx.Done()
			_ = srv.Close()
		}()
		_ = srv.Serve(ln)
	}()

	tlsCfg, err := tlsConfig(cfg)
	if err != nil {
		return err
	}

	ln, err := tls.Listen("tcp", cfg.Listen, tlsCfg)
	if err != nil {
		return err
	}
	log.Printf("listening on https://%s%s (domain=%s version=%s)",
		displayHost(cfg), cfg.Listen, cfg.Domain, cfg.Version)

	httpSrv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(c)
	}()
	err = httpSrv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func tlsConfig(cfg *config.Config) (*tls.Config, error) {
	if cfg.DevInsecureTLS {
		cert, err := selfSigned(cfg.Domain)
		if err != nil {
			return nil, err
		}
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
		}, nil
	}
	if err := os.MkdirAll(cfg.ACMECache, 0o700); err != nil {
		return nil, err
	}
	m := &autocert.Manager{
		Cache:      autocert.DirCache(cfg.ACMECache),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.Domain),
		Email:      cfg.ACMEEmail,
	}
	tlsCfg := m.TLSConfig()
	tlsCfg.MinVersion = tls.VersionTLS13
	return tlsCfg, nil
}

func selfSigned(cn string) (tls.Certificate, error) {
	if cn == "" {
		cn = "localhost"
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{cn, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}

func displayHost(cfg *config.Config) string {
	if cfg.Domain != "" {
		return cfg.Domain
	}
	return "localhost"
}
