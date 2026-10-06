#!/usr/bin/env bash
# Packages a Linux release of Black Fortress: the runtime built by
# build-runtime.sh plus a relocatable PostgreSQL, so a fresh machine (a
# cloud agent sandbox, a container, a CI runner) needs nothing else.
#
#   blackfortress-linux-<arch>/
#     bin/        bfd, bf, bf-checks, probod, probod-bootstrap
#     postgres/   PostgreSQL 16 (bin, lib, share)
#     library/    frameworks and Comp's control, policy and task templates
#     install.sh  installs into /opt/blackfortress (root) or ~/.local/opt
#
# Usage: fortress/scripts/package-linux.sh [amd64|arm64]   (default amd64)
# Output: fortress/bfd/dist/blackfortress-linux-<arch>.tar.gz (+ .sha256)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ARCH="${1:-amd64}"
DIST="$ROOT/fortress/bfd/dist"
SRC="$DIST/linux-$ARCH"
NAME="blackfortress-linux-$ARCH"
OUT="$DIST/$NAME"

for f in bfd bf bf-checks probod probod-bootstrap; do
  [[ -x "$SRC/$f" ]] || { echo "missing $SRC/$f: run fortress/scripts/build-runtime.sh linux-$ARCH first" >&2; exit 1; }
done

rm -rf "$OUT"
mkdir -p "$OUT/bin"
cp "$SRC"/{bfd,bf,bf-checks,probod,probod-bootstrap} "$OUT/bin/"
cp -R "$SRC/library" "$OUT/library"
"$ROOT/fortress/scripts/fetch-postgres.sh" "linux-$ARCH" "$DIST/postgres-linux-$ARCH"
cp -R "$DIST/postgres-linux-$ARCH" "$OUT/postgres"
cp "$ROOT/fortress/linux/install.sh" "$OUT/install.sh"
cp "$ROOT/fortress/linux/README.md" "$OUT/README.md"
chmod -R go+rX "$OUT"

tar -C "$DIST" -czf "$DIST/$NAME.tar.gz" "$NAME"
(cd "$DIST" && sha256sum "$NAME.tar.gz" > "$NAME.tar.gz.sha256")
echo "==> $DIST/$NAME.tar.gz ($(du -h "$DIST/$NAME.tar.gz" | cut -f1))"
