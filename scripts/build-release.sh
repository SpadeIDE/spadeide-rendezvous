#!/usr/bin/env bash
# Build SpadeIDE rendezvous release artifacts and stage them in github-release/
# for a GitHub Release drag-and-drop (same workflow as spadeide-agent / KubeSpade).
#
# Usage:
#   ./scripts/build-release.sh              # version 0.1.0 (default)
#   ./scripts/build-release.sh 0.2.0
#   ./scripts/build-release.sh v0.2.0       # leading v stripped
#   ./scripts/build-release.sh --version 0.2.0
#
# Env:
#   ARCH=amd64|arm64|all   (default: all)
#   SKIP_TAR=1             skip tarballs (binaries only — not staged)
#
# Output (wipe + refill each run):
#   github-release/
#     spade-rendezvous-${VERSION}-linux-amd64.tar.gz
#     spade-rendezvous-${VERSION}-linux-arm64.tar.gz
#     SHA256SUMS.txt
#
set -euo pipefail
shopt -s nullglob

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEFAULT_VERSION="0.1.0"
OUT="$ROOT/github-release"
DIST="$ROOT/dist/rendezvous"

VERSION="${DEFAULT_VERSION}"
case "${1:-}" in
  "") ;;
  --version)
    VERSION="${2:?version required}"
    ;;
  -h|--help)
    sed -n '2,26p' "$0"
    exit 0
    ;;
  *)
    VERSION="$1"
    ;;
esac

VERSION="${VERSION#v}"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.]+)?$ ]]; then
  echo "error: version must look like 0.2.0 or v0.2.0 (got: ${1:-$VERSION})" >&2
  exit 1
fi

stage() {
  local dest_name="$1"
  shift
  local src="" f
  for f in "$@"; do
    [[ -f "$f" ]] || continue
    src="$f"
    break
  done
  if [[ -z "$src" ]]; then
    echo "    ERROR: no artifact for $dest_name (looked for: $*)" >&2
    return 1
  fi
  cp -f "$src" "$OUT/$dest_name"
  echo "    + $dest_name  ←  $(basename "$src")"
}

echo "=== SpadeIDE rendezvous release ${VERSION} ==="
echo "    Staging dir: $OUT"
echo ""

rm -rf "$OUT"
mkdir -p "$OUT"

export VERSION

if [[ "${SKIP_TAR:-}" == "1" ]]; then
  echo "error: SKIP_TAR=1 leaves nothing to stage for a release" >&2
  exit 1
fi

echo "=== Build Linux tarballs (ARCH=${ARCH:-all}) ==="
KIND=tar "$ROOT/scripts/build-rendezvous.sh"

echo ""
echo "=== Stage → github-release/ ==="

raw="${ARCH:-all}"
case "$raw" in
  all|both|"") archs=(amd64 arm64) ;;
  arm64|aarch64) archs=(arm64) ;;
  amd64|x86_64|x64) archs=(amd64) ;;
  *) echo "Unsupported ARCH=$raw" >&2; exit 1 ;;
esac

for arch in "${archs[@]}"; do
  stage "spade-rendezvous-${VERSION}-linux-${arch}.tar.gz" \
    "$DIST/spade-rendezvous-linux-${arch}.tar.gz"
done

echo ""
echo "=== SHA256SUMS.txt ==="
(
  cd "$OUT"
  rm -f SHA256SUMS.txt
  for f in *; do
    [[ -f "$f" && "$f" != SHA256SUMS.txt ]] || continue
    shasum -a 256 "$f" >> SHA256SUMS.txt
  done
  if [[ -f SHA256SUMS.txt ]]; then
    sort -k2 -o SHA256SUMS.txt SHA256SUMS.txt
  fi
  echo "    $(wc -l < SHA256SUMS.txt | tr -d ' ') files"
  cat SHA256SUMS.txt
)

echo ""
echo "=== Done ==="
echo "    Drag everything from:"
echo "      $OUT"
echo "    onto the GitHub Release v${VERSION} assets."
echo ""
ls -lh "$OUT"
echo ""
echo "    Next:  ./scripts/build-release.sh 0.2.0"
