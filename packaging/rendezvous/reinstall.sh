#!/usr/bin/env bash
# Reinstall spade-rendezvous from this tarball. Keeps /etc/spade-rendezvous/env,
# ACME cache, and notice.json — only the binary and unit are replaced.
#
#   sudo ./reinstall.sh

set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
INSTALL="$HERE/install.sh"

usage() {
  cat <<'EOF'
Usage: sudo ./reinstall.sh

  Replaces /usr/local/bin/spade-rendezvous and the systemd unit from this
  directory. Config (/etc/spade-rendezvous/env), ACME cache, and
  notice.json are left alone (notice.json is created if missing).

  Requires an existing install (binary or unit on disk).
EOF
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
  "") ;;
  *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
esac

if [[ ! -x "$INSTALL" ]]; then
  echo "install.sh not found next to reinstall.sh" >&2
  exit 1
fi

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Re-running with sudo…"
  exec sudo --preserve-env=SUDO_USER "$0" "$@"
fi

unit_present() {
  [[ -f /etc/systemd/system/spade-rendezvous.service ]] \
    || [[ -f /lib/systemd/system/spade-rendezvous.service ]] \
    || [[ -f /usr/lib/systemd/system/spade-rendezvous.service ]]
}

bin_present() {
  [[ -x /usr/local/bin/spade-rendezvous ]]
}

state_present() {
  [[ -d /var/lib/spade-rendezvous ]] || [[ -d /etc/spade-rendezvous ]]
}

if ! unit_present && ! bin_present && ! state_present; then
  echo "No existing SpadeIDE rendezvous install found." >&2
  echo "First-time install:" >&2
  echo "  sudo ./install.sh" >&2
  exit 1
fi

was_active=0
was_enabled=0
if command -v systemctl >/dev/null 2>&1; then
  systemctl is-active --quiet spade-rendezvous 2>/dev/null && was_active=1 || true
  systemctl is-enabled --quiet spade-rendezvous 2>/dev/null && was_enabled=1 || true
fi

echo "==> Stopping rendezvous before replacing the binary…"
systemctl stop spade-rendezvous 2>/dev/null || true

# Preserve User=/Group= from the installed unit when present (custom --user-name).
USER_ARGS=()
if [[ -f /etc/systemd/system/spade-rendezvous.service ]]; then
  old_user="$(awk -F= '/^User=/{print $2; exit}' /etc/systemd/system/spade-rendezvous.service || true)"
  if [[ -n "${old_user:-}" && "$old_user" != "spade-rv" ]]; then
    USER_ARGS=(--user-name "$old_user")
    echo "==> Keeping system user: $old_user"
  fi
fi

echo "==> Reinstalling from this archive…"
"$INSTALL" "${USER_ARGS[@]}"

if [[ "$was_enabled" -eq 1 ]]; then
  systemctl enable spade-rendezvous 2>/dev/null || true
fi
if [[ "$was_active" -eq 1 || "$was_enabled" -eq 1 ]]; then
  echo "==> Starting spade-rendezvous…"
  systemctl restart spade-rendezvous
fi

hash -r 2>/dev/null || true
echo
echo "Reinstall done. Env and ACME cache were kept."
echo "  sudo systemctl status spade-rendezvous"
echo "  curl -sS https://\$SPADE_RV_DOMAIN/healthz"
