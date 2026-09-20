# SpadeIDE rendezvous

Introducer + blind relay for [SpadeIDE](https://github.com/SpadeIDE) remote access. Agents register while remote access is on; iPads look up a blinded handle and get a short-lived relay ticket. Nested TLS between tablet and agent carries the session — this server only sees ciphertext.

**License:** Apache License 2.0 (see [LICENSE](LICENSE)).

## Build

```bash
make build          # host binary + linux amd64/arm64 tarballs → dist/rendezvous/
make linux-amd
make linux-arm
make test
make release        # stage github-release/ for a GitHub Release drag-and-drop
```

## GitHub Release

Same flow as `spadeide-agent` / KubeSpade:

```bash
./scripts/build-release.sh 0.2.0
# → github-release/
#      spade-rendezvous-0.2.0-linux-amd64.tar.gz
#      spade-rendezvous-0.2.0-linux-arm64.tar.gz
#      SHA256SUMS.txt
```

Create a GitHub release tagged `v0.2.0` and drag everything from `github-release/` onto the assets.

## Deploy

```bash
scp dist/rendezvous/spade-rendezvous-linux-amd64.tar.gz user@vps:
ssh user@vps
tar xzf spade-rendezvous-linux-amd64.tar.gz
cd spade-rendezvous-linux-amd64
sudo ./install.sh
sudo nano /etc/spade-rendezvous/env   # SPADE_RV_DOMAIN + ACME email
sudo systemctl enable --now spade-rendezvous
```

Local smoke (no ACME):

```bash
SPADE_RV_DEV_INSECURE=1 SPADE_RV_LISTEN=:8443 SPADE_RV_DOMAIN=localhost \
  ./dist/rendezvous/spade-rendezvous serve
```

See [docs/RENDEZVOUS.md](docs/RENDEZVOUS.md).

## Module

Go module path: `github.com/spadeide/spade-rendezvous`
