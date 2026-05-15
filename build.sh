#!/usr/bin/env bash
# Builds the web UI and cross-compiles self-contained mongosync-ui binaries.
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-$(date +%Y.%m.%d)}"
OUT="dist"

echo "==> Building web UI"
( cd web && npm install --no-audit --no-fund && npm run build )

echo "==> Cross-compiling mongosync-ui ${VERSION}"
mkdir -p "$OUT"
rm -f "$OUT"/mongosync-ui-*

TARGETS=(
  "darwin/arm64"
  "darwin/amd64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

for target in "${TARGETS[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  out="$OUT/mongosync-ui-${os}-${arch}"
  [ "$os" = "windows" ] && out="${out}.exe"
  echo "    ${target} -> ${out}"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o "$out" ./cmd/mongosync-ui
done

echo "==> Done. Binaries in ${OUT}/"
ls -lh "$OUT"/mongosync-ui-*
