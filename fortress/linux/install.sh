#!/bin/sh
# Installs Black Fortress from this extracted release.
#
#   ./install.sh [PREFIX]
#
# As root: PREFIX defaults to /opt/blackfortress and bf/bfd are linked into
# /usr/local/bin. Otherwise ~/.local/opt/blackfortress and ~/.local/bin.
set -eu

src="$(cd "$(dirname "$0")" && pwd)"

if [ "$(id -u)" = 0 ]; then
  prefix="${1:-/opt/blackfortress}"
  bindir=/usr/local/bin
else
  prefix="${1:-$HOME/.local/opt/blackfortress}"
  bindir="$HOME/.local/bin"
fi

case "$prefix" in
  /|/usr|/usr/local|/opt|"$HOME"|"") echo "refusing to install into $prefix" >&2; exit 1 ;;
esac

if [ "$src" = "$prefix" ]; then
  echo "already installed in $prefix"
else
  mkdir -p "$(dirname "$prefix")"
  rm -rf "$prefix.new"
  mkdir "$prefix.new"
  cp -R "$src/bin" "$src/library" "$src/postgres" "$prefix.new/"
  # PostgreSQL runs as an unprivileged user when bfd is root; it must be
  # able to read the install.
  chmod -R go+rX "$prefix.new"
  rm -rf "$prefix"
  mv "$prefix.new" "$prefix"
fi

mkdir -p "$bindir"
ln -sf "$prefix/bin/bf" "$bindir/bf"
ln -sf "$prefix/bin/bfd" "$bindir/bfd"

echo "Black Fortress $("$prefix/bin/bf" version) installed in $prefix"
case ":$PATH:" in
  *":$bindir:"*) ;;
  *) echo "add $bindir to PATH" ;;
esac
echo "start it with: bf up"
