# SpadeIDE Rendezvous 1.0.0

Introducer + blind relay for SpadeIDE remote access. Agents register when remote is on; iPads look up and get a short-lived relay ticket. Nested TLS carries the session — this server only sees ciphertext.

## Changes

- **Safer registration** — a live agent slot cannot be silently taken over by another connection.
- **Ticket signing** — key rotation keeps the previous key valid briefly so in-flight tickets still verify.
- **Operator notice** — `/etc/spade-rendezvous/notice.json` is always installed (`enabled: false` by default). Set `"enabled": true` to show a banner on the next remote lookup (hot-reload, no restart). Optional version filters and download links.
- **`reinstall.sh`** — upgrade the binary and unit; keeps env, ACME cache, and `notice.json`.
- **Ops clarity** — `SPADE_RV_SESSION_MAX_BYTES` is per direction; rate limits treat `0` as unlimited. Extra knobs: `SPADE_RV_RATE_AGENT_PER_MIN`, `SPADE_RV_RATE_REFLEX_PER_MIN`.

## Install

```bash
tar xzf spade-rendezvous-1.0.0-linux-amd64.tar.gz
cd spade-rendezvous-linux-amd64
sudo ./install.sh
sudo nano /etc/spade-rendezvous/env    # SPADE_RV_DOMAIN + ACME email
sudo systemctl enable --now spade-rendezvous
```

## Upgrade

```bash
sudo ./reinstall.sh
```
