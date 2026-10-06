#!/usr/bin/env bash
# Downloads a relocatable PostgreSQL build from the zonky
# embedded-postgres-binaries Maven artifacts and unpacks it into DEST
# (bin/, lib/, share/). Black Fortress bundles it so nothing has to be
# installed system-wide.
#
# Usage: fortress/scripts/fetch-postgres.sh <os>-<arch> DEST
#   os: darwin | linux     arch: arm64 | amd64
# Env:   PG_VERSION   zonky artifact version (default 16.15.0)
set -euo pipefail

TARGET="${1:?usage: fetch-postgres.sh <os>-<arch> DEST}"
DEST="${2:?usage: fetch-postgres.sh <os>-<arch> DEST}"
PG_VERSION="${PG_VERSION:-16.15.0}"
MAVEN="https://repo1.maven.org/maven2/io/zonky/test/postgres"

os="${TARGET%-*}"
arch="${TARGET#*-}"
case "$os" in darwin|linux) ;; *) echo "unknown os '$os' (darwin, linux)" >&2; exit 1 ;; esac
case "$arch" in
  arm64) zarch="arm64v8" ;;
  amd64) zarch="amd64" ;;
  *) echo "unknown arch '$arch' (arm64, amd64)" >&2; exit 1 ;;
esac

if [[ -x "$DEST/bin/postgres" && -f "$DEST/.version" && "$(cat "$DEST/.version")" == "$PG_VERSION" ]]; then
  echo "==> PostgreSQL $PG_VERSION ($TARGET) already present in $DEST"
  exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

artifact="embedded-postgres-binaries-${os}-${zarch}"
url="$MAVEN/$artifact/$PG_VERSION/$artifact-$PG_VERSION.jar"

echo "==> Downloading $url"
curl -fsSL --retry 5 --retry-delay 5 -o "$tmp/pg.jar" "$url"

# Verify against Maven Central's published SHA-1.
curl -fsSL --retry 5 --retry-delay 5 -o "$tmp/pg.jar.sha1" "$url.sha1"
expected="$(awk '{print $1}' "$tmp/pg.jar.sha1")"
if command -v sha1sum >/dev/null 2>&1; then
  actual="$(sha1sum "$tmp/pg.jar" | awk '{print $1}')"
else
  actual="$(shasum -a 1 "$tmp/pg.jar" | awk '{print $1}')"
fi
if [[ "$expected" != "$actual" ]]; then
  echo "checksum mismatch for $url (expected $expected, got $actual)" >&2
  exit 1
fi
echo "    sha1 ok ($actual)"

# The jar holds a single postgres-<os>-<arch>.txz.
txz="$(unzip -Z1 "$tmp/pg.jar" | grep -E '\.txz$' | head -n1)"
if [[ -z "$txz" ]]; then
  echo "no .txz found inside $url" >&2
  exit 1
fi
unzip -q -o "$tmp/pg.jar" "$txz" -d "$tmp"

rm -rf "$DEST.partial"
mkdir -p "$DEST.partial"
tar -xJf "$tmp/$txz" -C "$DEST.partial"
if [[ ! -x "$DEST.partial/bin/postgres" || ! -x "$DEST.partial/bin/initdb" ]]; then
  echo "archive did not contain bin/postgres and bin/initdb" >&2
  exit 1
fi
echo "$PG_VERSION" > "$DEST.partial/.version"

rm -rf "$DEST"
mkdir -p "$(dirname "$DEST")"
mv "$DEST.partial" "$DEST"
if [[ "$os" == darwin ]] && command -v xattr >/dev/null 2>&1; then
  xattr -cr "$DEST" 2>/dev/null || true
fi
echo "==> PostgreSQL $PG_VERSION ($TARGET) -> $DEST"
