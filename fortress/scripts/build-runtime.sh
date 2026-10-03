#!/usr/bin/env bash
# Builds the Black Fortress runtime for macOS (or any GOOS/GOARCH) into
# fortress/bfd/dist/<os>-<arch>/: bfd, bf, probod, probod-bootstrap and the
# framework library. The macOS app bundles that directory.
#
# Usage: fortress/scripts/build-runtime.sh [darwin-arm64 darwin-amd64 linux-amd64 ...]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PROBO="$ROOT/probo"
BFD="$ROOT/fortress/bfd"
TARGETS=("$@")
[ ${#TARGETS[@]} -eq 0 ] && TARGETS=(darwin-arm64 darwin-amd64)

PROBOD_VERSION="$(sed -n 's/^## \[\{0,1\}v\{0,1\}\([0-9][0-9.]*\).*/\1/p' "$PROBO/cmd/probod/CHANGELOG.md" | head -1)"
BF_VERSION="${BF_VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"

# probod embeds the console and portal frontends; build them (and generated
# Go code) once through Probo's own Makefile.
if [ ! -d "$PROBO/node_modules" ]; then
  (cd "$PROBO" && npm ci --no-audit --no-fund)
fi
make -C "$PROBO" bin/probod bin/probod-bootstrap >/dev/null

for target in "${TARGETS[@]}"; do
  os="${target%-*}"
  arch="${target#*-}"
  out="$BFD/dist/$target"
  echo "→ $target"
  mkdir -p "$out/library/frameworks"

  (cd "$PROBO" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X 'main.version=${PROBOD_VERSION:-0.0.0}' -X 'main.env=prod'" \
    -o "$out/probod" ./cmd/probod)
  (cd "$PROBO" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" \
    -o "$out/probod-bootstrap" ./cmd/probod-bootstrap)

  for cmd in bfd bf; do
    (cd "$BFD" && CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
      -ldflags "-s -w -X 'blackfortress.dev/fortress/bfd/internal/runtime.Version=$BF_VERSION'" \
      -o "$out/$cmd" "./cmd/$cmd")
  done

  cp "$PROBO"/apps/console/public/data/frameworks/*.json "$out/library/frameworks/"
  cp "$ROOT"/fortress/library/frameworks/*.json "$out/library/frameworks/"
done

echo "done: $BFD/dist"
