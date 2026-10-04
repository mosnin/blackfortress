#!/usr/bin/env bash
# Builds the Black Fortress runtime for macOS (or any GOOS/GOARCH) into
# fortress/bfd/dist/<os>-<arch>/: bfd, bf, bf-checks, probod,
# probod-bootstrap and the control library. The macOS app bundles that
# directory.
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

# bf-checks bundles Comp's integration checks into a standalone Bun binary.
(cd "$ROOT/fortress/checks" && bun install --frozen-lockfile >/dev/null)

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

  case "$arch" in amd64) bun_arch=x64 ;; *) bun_arch="$arch" ;; esac
  (cd "$ROOT/fortress/checks" && bun build src/main.ts \
    --compile --minify --target="bun-$os-$bun_arch" --outfile "$out/bf-checks" >/dev/null)

  mkdir -p "$out/library/processes"
  cp "$PROBO"/apps/console/public/data/frameworks/*.json "$out/library/frameworks/"
  cp "$ROOT"/fortress/library/frameworks/*.json "$out/library/frameworks/"
  cp "$ROOT"/fortress/library/processes/*.json "$out/library/processes/"
done

echo "done: $BFD/dist"
