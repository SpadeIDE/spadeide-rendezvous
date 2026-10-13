# Rendezvous server

Introducer + blind relay for SpadeIDE remote access. Agents register while
**Remote access** is on; iPads look up a blinded hourly `rid` (or legacy
`agentId`) and get a short-lived relay ticket. Nested TLS 1.3 between iPad and
agent carries the real session — the server only sees ciphertext.

When hole punch works (typical home cone NAT), the agent also advertises a
reflexive UDP address; the iPad tries **Direct · QUIC** first, then falls back
to relay.

Architecture: product `docs/REMOTE_ACCESS.md` (private tree). Wire protocol:
`docs/PROTOCOL.md` (this repo).

## Build

```bash
make build          # → dist/rendezvous/
# or
./scripts/build-rendezvous.sh
```

Public checkout: sibling repo `spadeide-rendezvous` (this tree when developing there).

## Hosted product

Default URL baked into agent + iPad: `wss://rendezvous.spadeide.com`

```bash
curl -fsS https://rendezvous.spadeide.com/healthz   # ok
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

Upgrade from a newer tarball (keeps `/etc/spade-rendezvous/env` and ACME cache):

```bash
sudo ./reinstall.sh
```

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

## Operator notice

`install.sh` always writes `/etc/spade-rendezvous/notice.json` (default
`enabled: false`). Edit that file and set `"enabled": true` to broadcast —
hot-reloaded on mtime, no restart. Reinstall keeps an existing file.

```bash
sudo nano /etc/spade-rendezvous/notice.json
# set "enabled": true, fill title/body, save
```

| Field | Meaning |
|-------|---------|
| `enabled` | `false` (default) = silent; `true` = attach to lookups |
| `id` | Stable key; clients dismiss once per id |
| `title` / `body` | Banner text (`id` + at least one of these required when enabled) |
| `severity` | `info` (default), `warning`, or `critical` |
| `belowAppVersion` | Show only if the iPad app version is **strictly below** this (empty = ignore) |
| `belowAgentVersion` | Show only if any paired host’s last-known agent version is below |
| `agentUrl` | Optional `https://` link — host agent download page |
| `appUrl` | Optional `https://` link — iPad / App Store |

If both version fields are empty, every client that looks up sees the notice.
`GET /v1/notice` returns 204 while disabled. Path override:
`SPADE_RV_NOTICE_FILE` (default `/etc/spade-rendezvous/notice.json`).
File capped at **16 KiB**; non-`https` URLs reject the whole document.

## Ops notes

- WebSocket frames are capped (`64 KiB` control, `1 MiB` relay). Session defaults:
  **512 MiB per direction** / **8 h** (`SPADE_RV_SESSION_MAX_*`); a duplex
  session can therefore move up to **2×** the byte cap. `0` bytes = unlimited.
  Bad env values abort startup. Wire shapes: `docs/PROTOCOL.md`.
- systemd unit ships full §6.8 hardening. Native clients omit `Origin`; browser
  Origins are rejected.
- **Progressive IP bans** on `/v1/agent`, `/v1/client`, `/v1/relay` (not shared
  with UDP reflex — spoofable sources only soft-drop):
  - Soft windows (defaults): lookups **60**/min, agent register **30**/min,
    reflex **120**/min. `SPADE_RV_RATE_*=0` means **unlimited**.
  - Rate soft-trips cap at **15m**. Auth/protocol floods after a soft window
    can climb **1m → 5m → 15m → 1h → 24h**.
  - Banbook entries are retained up to **24h** after the last strike (strike
    decay); a restart clears all in-memory state.
  - Ban checks run **before** WebSocket upgrade (cheap 429 + `Retry-After`).
    Reflex over-limit drops silently (no amplification reply).
- Metrics (loopback): `spade_rv_bans_active`, `spade_rv_ban_strikes_total`,
  `spade_rv_ban_rejected_total`, `spade_rv_ban_evictions_total`.
- `make test` / `make lint` / `make vuln` — unit tests, golangci-lint, govulncheck.
