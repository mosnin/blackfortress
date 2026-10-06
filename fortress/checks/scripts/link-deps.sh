#!/usr/bin/env sh
# Comp's integration-platform source imports zod and the AWS SDK. In this
# repository Comp's workspace isn't installed, so point its node_modules at
# this package's dependencies. A real Comp install (a directory) is left alone.
set -e
here="$(cd "$(dirname "$0")/.." && pwd)"
target="$here/../../packages/integration-platform/node_modules"
if [ -e "$target" ] && [ ! -L "$target" ]; then
  exit 0
fi
ln -sfn "$here/node_modules" "$target"
