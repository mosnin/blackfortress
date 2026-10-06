#!/usr/bin/env bash
# Fetches the relocatable PostgreSQL 16 build for macOS that the app bundles,
# into Resources/postgres/darwin-<arch>/{bin,lib,share}. The download and
# verification live in fortress/scripts/fetch-postgres.sh (shared with the
# Linux release).
#
# Usage: scripts/fetch-postgres.sh [arm64|amd64|all]   (default: host arch)
# Env:   PG_VERSION   zonky artifact version (default 16.15.0)
#        PG_OUT_ROOT  output root (default <macos>/Resources/postgres)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_ROOT="${PG_OUT_ROOT:-$ROOT/Resources/postgres}"
FETCH="$ROOT/../scripts/fetch-postgres.sh"

host_arch() {
  case "$(uname -m)" in
    arm64|aarch64) echo arm64 ;;
    x86_64|amd64) echo amd64 ;;
    *) echo "unsupported host arch: $(uname -m)" >&2; exit 1 ;;
  esac
}

case "${1:-}" in
  "") archs=("$(host_arch)") ;;
  all) archs=(arm64 amd64) ;;
  *) archs=("$1") ;;
esac

for arch in "${archs[@]}"; do
  "$FETCH" "darwin-$arch" "$OUT_ROOT/darwin-$arch"
done
