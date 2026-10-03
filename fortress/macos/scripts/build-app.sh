#!/usr/bin/env bash
# Builds BlackFortress.app without Xcode:
#   1. swift build -c release
#   2. assembles build/BlackFortress.app (Info.plist, executable)
#   3. bundles bfd, bf, probod, probod-bootstrap from ../bfd/dist/darwin-<arch>
#      into Contents/Resources/bin, and the control library (dist/.../library)
#      into Contents/Resources/library (bfd resolves <exe dir>/../library/frameworks).
#      The dist directory is produced by fortress/scripts/build-runtime.sh.
#   4. bundles PostgreSQL 16 into Contents/Resources/postgres
#   5. ad-hoc codesigns everything
#
# Env:
#   VERSION         CFBundleShortVersionString (default 0.1.0)
#   BUILD_NUMBER    CFBundleVersion (default: git commit count or 1)
#   BFD_DIST        directory with bfd/bf/probod/probod-bootstrap and library/
#                   (default ../bfd/dist/darwin-<arch>)
#   PG_SRC          PostgreSQL dir with bin/ lib/ share/
#                   (default Resources/postgres/darwin-<arch>; fetched if missing)
#   SKIP_PG=1       do not bundle PostgreSQL
#   STRICT=1        fail if any runtime binary is missing
#   OUT_DIR         output directory (default build)
#   SIGN_IDENTITY   codesign identity (default "-" = ad-hoc)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "build-app.sh must run on macOS (needs the macOS SDK and codesign)." >&2
  exit 1
fi

case "$(uname -m)" in
  arm64) GOARCH=arm64 ;;
  x86_64) GOARCH=amd64 ;;
  *) echo "unsupported arch $(uname -m)" >&2; exit 1 ;;
esac

APP_NAME="Black Fortress"
EXEC_NAME="BlackFortress"
BUNDLE_ID="dev.blackfortress.app"
VERSION="${VERSION:-0.1.0}"
BUILD_NUMBER="${BUILD_NUMBER:-$(git rev-list --count HEAD 2>/dev/null || echo 1)}"
BFD_DIST="${BFD_DIST:-$ROOT/../bfd/dist/darwin-$GOARCH}"
PG_SRC="${PG_SRC:-$ROOT/Resources/postgres/darwin-$GOARCH}"
OUT_DIR="${OUT_DIR:-$ROOT/build}"
SIGN_IDENTITY="${SIGN_IDENTITY:--}"
APP="$OUT_DIR/$EXEC_NAME.app"

echo "==> swift build -c release"
swift build -c release
BIN_PATH="$(swift build -c release --show-bin-path)"

echo "==> Assembling $APP"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/bin"
cp "$BIN_PATH/$EXEC_NAME" "$APP/Contents/MacOS/$EXEC_NAME"

# SwiftPM resource bundles (none today, but keep the app working if added).
for bundle in "$BIN_PATH"/*.bundle; do
  [[ -e "$bundle" ]] || continue
  ditto "$bundle" "$APP/Contents/Resources/$(basename "$bundle")"
done

if [[ -f "$ROOT/Resources/AppIcon.icns" ]]; then
  cp "$ROOT/Resources/AppIcon.icns" "$APP/Contents/Resources/AppIcon.icns"
fi

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleDisplayName</key>
  <string>$APP_NAME</string>
  <key>CFBundleName</key>
  <string>$APP_NAME</string>
  <key>CFBundleExecutable</key>
  <string>$EXEC_NAME</string>
  <key>CFBundleIdentifier</key>
  <string>$BUNDLE_ID</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>$VERSION</string>
  <key>CFBundleVersion</key>
  <string>$BUILD_NUMBER</string>
  <key>CFBundleIconFile</key>
  <string>AppIcon</string>
  <key>LSMinimumSystemVersion</key>
  <string>14.0</string>
  <key>LSApplicationCategoryType</key>
  <string>public.app-category.developer-tools</string>
  <key>LSUIElement</key>
  <false/>
  <key>NSHighResolutionCapable</key>
  <true/>
  <key>NSPrincipalClass</key>
  <string>NSApplication</string>
  <key>NSHumanReadableCopyright</key>
  <string>Black Fortress — local compliance runtime</string>
  <key>NSAppTransportSecurity</key>
  <dict>
    <key>NSAllowsLocalNetworking</key>
    <true/>
    <key>NSExceptionDomains</key>
    <dict>
      <key>localhost</key>
      <dict>
        <key>NSExceptionAllowsInsecureHTTPLoads</key>
        <true/>
        <key>NSIncludesSubdomains</key>
        <false/>
      </dict>
      <key>127.0.0.1</key>
      <dict>
        <key>NSExceptionAllowsInsecureHTTPLoads</key>
        <true/>
        <key>NSIncludesSubdomains</key>
        <false/>
      </dict>
    </dict>
  </dict>
</dict>
</plist>
PLIST
plutil -lint "$APP/Contents/Info.plist" >/dev/null
printf 'APPL????' > "$APP/Contents/PkgInfo"

echo "==> Bundling runtime binaries from $BFD_DIST"
missing=0
for b in bfd bf probod probod-bootstrap; do
  if [[ -f "$BFD_DIST/$b" ]]; then
    cp "$BFD_DIST/$b" "$APP/Contents/Resources/bin/$b"
    chmod 755 "$APP/Contents/Resources/bin/$b"
    echo "    + $b"
  else
    echo "    ! missing $BFD_DIST/$b" >&2
    missing=1
  fi
done
if [[ -d "$BFD_DIST/library" ]]; then
  ditto "$BFD_DIST/library" "$APP/Contents/Resources/library"
  n_fw=$(find "$APP/Contents/Resources/library/frameworks" -name '*.json' 2>/dev/null | wc -l | tr -d ' ')
  echo "    + library/ ($n_fw framework files)"
else
  echo "    ! missing $BFD_DIST/library (control library; bfd will start without frameworks)" >&2
  missing=1
fi
if [[ "$missing" == 1 ]]; then
  if [[ "${STRICT:-0}" == 1 ]]; then
    echo "runtime binaries missing (STRICT=1)" >&2
    exit 1
  fi
  echo "    warning: incomplete runtime bundle; build it with fortress/scripts/build-runtime.sh" >&2
fi

if [[ "${SKIP_PG:-0}" != 1 ]]; then
  if [[ ! -x "$PG_SRC/bin/postgres" ]]; then
    echo "==> PostgreSQL not found at $PG_SRC, fetching"
    "$ROOT/scripts/fetch-postgres.sh" "$GOARCH"
  fi
  echo "==> Bundling PostgreSQL from $PG_SRC"
  ditto "$PG_SRC" "$APP/Contents/Resources/postgres"
fi

echo "==> Codesigning ($SIGN_IDENTITY)"
xattr -cr "$APP" 2>/dev/null || true
# Sign nested Mach-O files first (executables, dylibs, extension modules),
# then the app itself. --deep is avoided on purpose.
while IFS= read -r -d '' f; do
  if file -b "$f" | grep -q 'Mach-O'; then
    codesign --force --sign "$SIGN_IDENTITY" --timestamp=none "$f"
  fi
done < <(find "$APP/Contents/Resources" -type f \( -perm -u+x -o -name '*.dylib' -o -name '*.so' \) -print0)
codesign --force --sign "$SIGN_IDENTITY" --timestamp=none "$APP"
codesign --verify --strict "$APP" && echo "    signature ok"

echo "==> Done: $APP"
