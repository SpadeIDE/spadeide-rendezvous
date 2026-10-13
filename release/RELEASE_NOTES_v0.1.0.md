# SpadeIDE Rendezvous 0.1.0

Introducer + blind relay for SpadeIDE remote access. Agents register when remote is on; iPads look up and get a short-lived relay ticket. Nested TLS carries the session — this server only sees ciphertext.

## Highlights

- **Faster lookup path** — relay ticket without waiting on UDP reflex; optional late Direct candidate.
- **`reinstall.sh`** — replace the binary and unit; keeps `/etc/spade-rendezvous/env`, ACME cache, and `notice.json`.
- **Operator notice** — `/etc/spade-rendezvous/notice.json` is always installed (`enabled: false` by default). Set `"enabled": true` to broadcast a banner to iPads on the next remote lookup (hot-reload, no restart). Optional version filters and `agentUrl` / `appUrl` links.
- **Hardening** — WebSocket size caps, finite session limits, progressive IP bans, systemd lockdown, reject browser `Origin`.

## Install

```bash
tar xzf spade-rendezvous-0.1.0-linux-amd64.tar.gz
cd spade-rendezvous-linux-amd64
sudo ./install.sh
sudo nano /etc/spade-rendezvous/env    # SPADE_RV_DOMAIN + ACME email
sudo systemctl enable --now spade-rendezvous
```

## Upgrade

```bash
sudo ./reinstall.sh
```

## Operator notice

Edit `/etc/spade-rendezvous/notice.json` — set `"enabled": true` when you want clients to see it. Check with `curl -sS https://YOUR_DOMAIN/v1/notice`.

See `SHA256SUMS.txt` in this release. Matching agent + iPad builds recommended.
