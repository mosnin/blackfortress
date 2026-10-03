#!/usr/bin/env bash
# Downloads a relocatable PostgreSQL 16 build for macOS from the zonky
# embedded-postgres-binaries Maven artifacts and unpacks it into
#   Resources/postgres/darwin-<arch>/{bin,lib,share}
#
# Usage: scripts/fetch-postgres.sh [arm64|amd64|all]   (default: host arch)
# Env:   PG_VERSION   zonky artifact version (default 16.15.0)
#        PG_OUT_ROOT  output root (default <macos>/Resources/postgres)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PG_VERSION="${PG_VERSION:-16.15.0}"
OUT_ROOT="${PG_OUT_ROOT:-$ROOT/Resources/postgres}"
MAVEN="https://repo1.maven.org/maven2/io/zonky/test/postgres"

host_arch() {
  case "$(uname -m)" in
    arm64|aarch64) echo arm64 ;;
    x86_64|amd64) echo amd64 ;;
    *) echo "unsupported host arch: $(uname -m)" >&2; exit 1 ;;
  esac
}

fetch_one() {
  local arch="$1" zarch
  case "$arch" in
    arm64) zarch="arm64v8" ;;
    amd64) zarch="amd64" ;;
    *) echo "unknown arch '$arch' (use arm64 or amd64)" >&2; exit 1 ;;
  esac

  local artifact="embedded-postgres-binaries-darwin-${zarch}"
  local url="$MAVEN/$artifact/$PG_VERSION/$artifact-$PG_VERSION.jar"
  local dest="$OUT_ROOT/darwin-$arch"

  if [[ -x "$dest/bin/postgres" && -f "$dest/.version" && "$(cat "$dest/.version")" == "$PG_VERSION" ]]; then
    echo "==> PostgreSQL $PG_VERSION ($arch) already present in $dest"
    return
  fi

  local tmp="$TMP_ROOT/$arch"
  mkdir -p "$tmp"

  echo "==> Downloading $url"
  curl -fsSL --retry 3 -o "$tmp/pg.jar" "$url"

  # Verify against Maven Central's published SHA-1 when available.
  if curl -fsSL -o "$tmp/pg.jar.sha1" "$url.sha1" 2>/dev/null; then
    local expected actual
    expected="$(awk '{print $1}' "$tmp/pg.jar.sha1")"
    if command -v shasum >/dev/null 2>&1; then
      actual="$(shasum -a 1 "$tmp/pg.jar" | awk '{print $1}')"
    else
      actual="$(sha1sum "$tmp/pg.jar" | awk '{print $1}')"
    fi
    if [[ "$expected" != "$actual" ]]; then
      echo "checksum mismatch for $url (expected $expected, got $actual)" >&2
      exit 1
    fi
    echo "    sha1 ok ($actual)"
  fi

  # The jar contains a single postgres-darwin-<arch>.txz.
  local txz
  txz="$(unzip -Z1 "$tmp/pg.jar" | grep -E '\.txz$' | head -n1)"
  if [[ -z "$txz" ]]; then
    echo "no .txz found inside $url" >&2
    exit 1
  fi
  unzip -q -o "$tmp/pg.jar" "$txz" -d "$tmp"

  rm -rf "$dest.partial"
  mkdir -p "$dest.partial"
  tar -xJf "$tmp/$txz" -C "$dest.partial"
  if [[ ! -x "$dest.partial/bin/postgres" ]]; then
    echo "archive did not contain bin/postgres" >&2
    exit 1
  fi
  echo "$PG_VERSION" > "$dest.partial/.version"

  rm -rf "$dest"
  mv "$dest.partial" "$dest"
  if command -v xattr >/dev/null 2>&1; then
    xattr -cr "$dest" 2>/dev/null || true
  fi
  echo "==> PostgreSQL $PG_VERSION ($arch) -> $dest"
  ls "$dest/bin"
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

mkdir -p "$OUT_ROOT"
case "${1:-}" in
  "") fetch_one "$(host_arch)" ;;
  all) fetch_one arm64; fetch_one amd64 ;;
  *) fetch_one "$1" ;;
esac
