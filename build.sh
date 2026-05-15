#!/usr/bin/env bash
# Builds the web UI and cross-compiles self-contained mongosync-ui release
# binaries for every supported platform into ./releases.
#
# The release version is read from the VERSION file (override with VERSION=…).
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-$(cat VERSION 2>/dev/null || echo dev)}"
OUT="releases"

echo "==> Building web UI"
( cd web && npm install --no-audit --no-fund && npm run build )

echo "==> Cross-compiling mongosync-ui ${VERSION}"
mkdir -p "$OUT"
rm -f "$OUT"/mongosync-ui-* "$OUT"/SHA256SUMS

TARGETS=(
  "darwin/arm64"
  "darwin/amd64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

for target in "${TARGETS[@]}"; do
  goos="${target%/*}"
  arch="${target#*/}"
  # Label darwin builds "macos" in the filename (GOOS stays "darwin").
  label="$goos"
  [ "$goos" = "darwin" ] && label="macos"
  out="$OUT/mongosync-ui-${VERSION}-${label}-${arch}"
  [ "$goos" = "windows" ] && out="${out}.exe"
  echo "    ${target} -> ${out}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$out" ./cmd/mongosync-ui
done

echo "==> Writing checksums"
( cd "$OUT" && shasum -a 256 mongosync-ui-* > SHA256SUMS )

echo "==> Done. Release ${VERSION} in ${OUT}/"
ls -lh "$OUT"
