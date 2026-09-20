#!/usr/bin/env bash
# Install spade-rendezvous from this directory (tarball contents).
#
#   sudo ./install.sh
#   sudo ./install.sh --user-name spade-rv
#
# Edit /etc/spade-rendezvous/env, then:
#   sudo systemctl enable --now spade-rendezvous

set -euo pipefail

SYS_USER="${SPADE_RV_USER:-spade-rv}"
HERE="$(cd "$(dirname "$0")" && pwd)"

usage() {
  cat <<'EOF'
Usage: sudo ./install.sh [--user-name NAME]

  Installs system-wide:
    /usr/local/bin/spade-rendezvous
    /etc/spade-rendezvous/env          (from env.example if missing)
    /etc/systemd/system/spade-rendezvous.service

  --user-name    System account (default: spade-rv)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --user-name) SYS_USER="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo ./install.sh" >&2
  exit 1
fi

host_arch="$(uname -m)"
case "$host_arch" in
  x86_64|amd64) want="amd64" ;;
  arm64|aarch64) want="arm64" ;;
  *) echo "Unsupported arch: $host_arch (need amd64 or arm64)" >&2; exit 1 ;;
esac

find_binary() {
  local c
  for c in \
    "$HERE/spade-rendezvous" \
    "$HERE/spade-rendezvous-linux-$want" \
    "$HERE/../../dist/rendezvous/spade-rendezvous-linux-$want"
  do
    if [[ -f "$c" && -x "$c" ]]; then
      echo "$c"
      return 0
    fi
  done
  return 1
}

BIN="$(find_binary)" || {
  echo "spade-rendezvous binary not found. Build first:" >&2
  echo "  make rendezvous" >&2
  echo "or unpack the linux-$want tarball and run install.sh from there." >&2
  exit 1
}

UNIT="$HERE/spade-rendezvous.service"
ENV_EX="$HERE/env.example"
if [[ ! -f "$UNIT" ]]; then
  UNIT="$HERE/../../packaging/rendezvous/spade-rendezvous.service"
  ENV_EX="$HERE/../../packaging/rendezvous/env.example"
fi

echo "==> Installing spade-rendezvous from $(basename "$BIN") ($want)"

if ! id -u "$SYS_USER" >/dev/null 2>&1; then
  echo "==> Creating user $SYS_USER"
  useradd --system --home-dir /var/lib/spade-rendezvous --create-home \
    --shell /usr/sbin/nologin "$SYS_USER" 2>/dev/null \
    || useradd --system --home-dir /var/lib/spade-rendezvous --create-home \
      --shell /bin/false "$SYS_USER"
fi

install -m 0755 "$BIN" /usr/local/bin/spade-rendezvous
mkdir -p /etc/spade-rendezvous /var/lib/spade-rendezvous/acme
chown -R "$SYS_USER:$SYS_USER" /var/lib/spade-rendezvous

if [[ ! -f /etc/spade-rendezvous/env ]]; then
  if [[ -f "$ENV_EX" ]]; then
    install -m 0640 "$ENV_EX" /etc/spade-rendezvous/env
  else
    cat >/etc/spade-rendezvous/env <<'EOF'
SPADE_RV_DOMAIN=rendezvous.example.com
SPADE_RV_ACME_EMAIL=ops@example.com
SPADE_RV_LISTEN=:443
SPADE_RV_METRICS_LISTEN=127.0.0.1:9090
SPADE_RV_ACME_CACHE=/var/lib/spade-rendezvous/acme
EOF
    chmod 0640 /etc/spade-rendezvous/env
  fi
  chown root:"$SYS_USER" /etc/spade-rendezvous/env
  echo "==> Wrote /etc/spade-rendezvous/env — edit SPADE_RV_DOMAIN before start"
else
  echo "==> Keeping existing /etc/spade-rendezvous/env"
fi

# Patch User= in unit to match --user-name
tmp_unit="$(mktemp)"
sed "s/^User=.*/User=$SYS_USER/; s/^Group=.*/Group=$SYS_USER/" "$UNIT" >"$tmp_unit"
install -m 0644 "$tmp_unit" /etc/systemd/system/spade-rendezvous.service
rm -f "$tmp_unit"

systemctl daemon-reload
echo ""
echo "Next:"
echo "  sudo nano /etc/spade-rendezvous/env"
echo "  sudo systemctl enable --now spade-rendezvous"
echo "  sudo systemctl status spade-rendezvous"
echo "  curl -sS https://\$SPADE_RV_DOMAIN/healthz"
