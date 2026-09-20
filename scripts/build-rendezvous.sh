#!/usr/bin/env bash
# Build spade-rendezvous → dist/rendezvous/
#
# Usage:
#   ./scripts/build-rendezvous.sh              # linux amd64+arm64 tarballs + host binary
#   ARCH=amd64 ./scripts/build-rendezvous.sh   # one arch
#   ARCH=arm64 ./scripts/build-rendezvous.sh
#   KIND=bin ./scripts/build-rendezvous.sh     # binaries only (no tarballs)
#   KIND=tar ./scripts/build-rendezvous.sh     # tarballs only (skip host-native)
#
# Output:
#   dist/rendezvous/spade-rendezvous                 (host OS, local smoke)
#   dist/rendezvous/spade-rendezvous-linux-amd64
#   dist/rendezvous/spade-rendezvous-linux-arm64
#   dist/rendezvous/spade-rendezvous-linux-amd64.tar.gz
#   dist/rendezvous/spade-rendezvous-linux-arm64.tar.gz
#
# Deploy:
#   scp dist/rendezvous/spade-rendezvous-linux-amd64.tar.gz user@host:
#   tar xzf spade-rendezvous-linux-amd64.tar.gz && cd spade-rendezvous-linux-amd64
#   sudo ./install.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${OUT_DIR:-$ROOT/dist/rendezvous}"
PKG="$ROOT/packaging/rendezvous"
VERSION="${VERSION:-0.1.0}"
KIND="${KIND:-all}"

raw="${ARCH:-all}"
case "$raw" in
  all|both|"") archs=(amd64 arm64) ;;
  arm64|aarch64) archs=(arm64) ;;
  amd64|x86_64|x64) archs=(amd64) ;;
  *) echo "Unsupported ARCH=$raw (use amd64, arm64, or all)" >&2; exit 1 ;;
esac

case "$KIND" in
  all|both|""|bin|tar) ;;
  *) echo "Unsupported KIND=$KIND (use bin, tar, or all)" >&2; exit 1 ;;
esac

mkdir -p "$OUT"
cd "$ROOT/server"

LDFLAGS="-s -w -X main.Version=$VERSION"

build_linux() {
  local goarch="$1"
  local name="spade-rendezvous-linux-${goarch}"
  echo "==> $name"
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build -trimpath \
    -ldflags "$LDFLAGS" \
    -o "$OUT/$name" ./cmd/spade-rendezvous
}

pack_tar() {
  local goarch="$1"
  local name="spade-rendezvous-linux-${goarch}"
  local bin="$OUT/$name"
  local stage="$OUT/.stage-$goarch"
  local tar="$OUT/${name}.tar.gz"

  [[ -f "$bin" ]] || { echo "missing $bin" >&2; exit 1; }

  rm -rf "$stage"
  mkdir -p "$stage/$name"
  install -m 0755 "$bin" "$stage/$name/spade-rendezvous"
  install -m 0755 "$PKG/install.sh" "$stage/$name/install.sh"
  install -m 0644 "$PKG/spade-rendezvous.service" "$stage/$name/spade-rendezvous.service"
  install -m 0644 "$PKG/env.example" "$stage/$name/env.example"
  cat >"$stage/$name/README.txt" <<EOF
SpadeIDE rendezvous $VERSION (linux/$goarch)

  sudo ./install.sh
  sudo nano /etc/spade-rendezvous/env   # set SPADE_RV_DOMAIN + ACME email
  sudo systemctl enable --now spade-rendezvous

Local smoke (no ACME):
  SPADE_RV_DEV_INSECURE=1 SPADE_RV_LISTEN=:8443 SPADE_RV_DOMAIN=localhost \\
    ./spade-rendezvous serve
EOF

  tar -C "$stage" -czf "$tar" "$name"
  rm -rf "$stage"
  echo "    $tar"
}

# Linux targets
for a in "${archs[@]}"; do
  if [[ "$KIND" == "all" || "$KIND" == "both" || "$KIND" == "" || "$KIND" == "bin" || "$KIND" == "tar" ]]; then
    build_linux "$a"
  fi
  if [[ "$KIND" == "all" || "$KIND" == "both" || "$KIND" == "" || "$KIND" == "tar" ]]; then
    pack_tar "$a"
  fi
done

# Host-native for local smoke (skip when KIND=tar)
if [[ "$KIND" != "tar" ]]; then
  echo "==> spade-rendezvous (host)"
  go build -trimpath -ldflags "$LDFLAGS" \
    -o "$OUT/spade-rendezvous" ./cmd/spade-rendezvous
fi

echo ""
echo "==> Done ($VERSION)"
ls -lh "$OUT"/spade-rendezvous* 2>/dev/null | sed 's/^/  /'
echo ""
echo "Deploy example:"
echo "  scp $OUT/spade-rendezvous-linux-amd64.tar.gz user@vps:"
echo "  ssh user@vps 'tar xzf spade-rendezvous-linux-amd64.tar.gz && cd spade-rendezvous-linux-amd64 && sudo ./install.sh'"
