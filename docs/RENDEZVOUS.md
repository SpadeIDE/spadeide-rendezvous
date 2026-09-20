# Rendezvous server

Introducer + blind relay for SpadeIDE remote access. Agents register while
**Remote access** is on; iPads look up a blinded hourly `rid` (or legacy
`agentId`) and get a short-lived relay ticket. Nested TLS 1.3 between iPad and
agent carries the real session — the server only sees ciphertext.

When hole punch works (typical home cone NAT), the agent also advertises a
reflexive UDP address; the iPad tries **Direct · QUIC** first, then falls back
to relay.

Architecture: `docs/REMOTE_ACCESS.md`. Wire protocol: `docs/PROTOCOL.md`.

## Build

```bash
make build          # → dist/rendezvous/
# or
./scripts/build-rendezvous.sh
```

Public checkout: sibling repo `spadeide-rendezvous` (this tree when developing there).

## Hosted product

Default URL baked into agent + iPad: `wss://openvpn.profinado.com`

```bash
curl -fsS https://openvpn.profinado.com/healthz   # ok
```

Open **UDP 443** on the host firewall for reflexive echo (same port as TLS).

## Self-host (Ubuntu)

See `packaging/rendezvous/`. Minimal:

```bash
export SPADE_RV_DOMAIN=rv.example.com
export SPADE_RV_ACME_EMAIL=you@example.com
# SPADE_RV_LISTEN=:443
# SPADE_RV_UDP_LISTEN=:443

./dist/rendezvous/spade-rendezvous serve
```

Point the agent: `spadeide remote on --server wss://rv.example.com`  
Point the iPad: **Settings → Remote → Rendezvous URL** (empty = product host).

## Local smoke (self-signed)

```bash
SPADE_RV_DEV_INSECURE=1 SPADE_RV_LISTEN=:8443 SPADE_RV_UDP_LISTEN=:8443 \
  SPADE_RV_DOMAIN=localhost \
  ./dist/rendezvous/spade-rendezvous serve
```

Agent/iPad accept insecure TLS only for `localhost` / `127.0.0.1` / `::1`.

## Path labels (iPad status)

| Label | Meaning |
|-------|---------|
| QUIC · mux | Same LAN |
| Direct · QUIC | Hole-punched internet UDP |
| Relay · mux | Rendezvous byte pipe + nested TLS |
| WSS · single | LAN WebSocket fallback |

## What this does *not* do

- Full ICE / TURN for symmetric NAT (those stay on relay)
- True in-place splice of Relay bytes into QUIC (upgrade is a soft session swap)
- Per-device `rvSecret` (v1 is agent-scoped; all paired iPads share one handle)

## Ops notes

- WebSocket frames are capped (`64 KiB` control, `1 MiB` relay). Session defaults:
  **512 MiB** / **8 h** (`SPADE_RV_SESSION_MAX_*`); bad env values abort startup.
- systemd unit ships full §6.8 hardening. Native clients omit `Origin`; browser
  Origins are rejected.
- `make rendezvous-test` / `make rendezvous-vuln` — unit+fuzz and govulncheck.
