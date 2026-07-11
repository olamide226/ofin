#!/bin/bash
# Build a standalone macOS .dmg for Òfin.
# Usage: bash create-dmg.sh <version>
#
# Bundles everything needed to run on ANY Apple Silicon Mac — no Homebrew,
# no system llama.cpp. The official llama.cpp release (binary + dylibs) is
# bundled under Resources/llama/. Only the 1.9 GB model downloads on first
# launch.
#
# Prerequisites (set via environment or edit below):
#   OFIN_BIN     path to ofin-darwin-arm64  (engine/bin/ofin-darwin-arm64)
#   LLAMA_DIR    path to extracted official llama.cpp release folder
#   EMBED_MODEL  path to bge-small-en-v1.5-f16.gguf
#   OFIN_DB      path to data/ofin.db

set -euo pipefail
VERSION="${1:-0.2.0}"
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"

OFIN_BIN="${OFIN_BIN:-$REPO/engine/bin/ofin-darwin-arm64}"
LLAMA_DIR="${LLAMA_DIR:-$HOME/.local/opt/llama.cpp}"
EMBED_MODEL="${EMBED_MODEL:-$REPO/models-dev/bge-small-en-v1.5-f16.gguf}"
OFIN_DB="${OFIN_DB:-$REPO/data/ofin.db}"
ICON="$HERE/../icons/ofin.icns"

APP="$HERE/Ofin.app"
RES="$APP/Contents/Resources"
DMG="$HERE/Ofin-${VERSION}-darwin-arm64.dmg"
STAGING="$HERE/dmg-staging"

echo "=== Òfin macOS DMG Builder (standalone) ==="

# Verify inputs
for f in "$OFIN_BIN" "$EMBED_MODEL" "$OFIN_DB" "$ICON"; do
  [ -f "$f" ] || { echo "ERROR: missing $f"; exit 1; }
done
[ -x "$LLAMA_DIR/llama-server" ] || { echo "ERROR: llama-server not found in $LLAMA_DIR"; exit 1; }

# Assemble Resources
echo "Assembling app bundle..."
rm -rf "$RES/llama"
mkdir -p "$RES/llama"
cp "$OFIN_BIN" "$RES/ofin-darwin-arm64"
cp "$EMBED_MODEL" "$RES/bge-small-en-v1.5-f16.gguf"
cp "$OFIN_DB" "$RES/ofin.db"
cp "$ICON" "$RES/ofin.icns"
echo "$VERSION" > "$RES/VERSION"

# Bundle the official llama.cpp: llama-server + every dylib it needs.
cp "$LLAMA_DIR/llama-server" "$RES/llama/"
cp "$LLAMA_DIR/"*.dylib "$RES/llama/"

chmod +x "$APP/Contents/MacOS/ofin-launcher" "$RES/ofin-darwin-arm64" "$RES/llama/llama-server"

# Sanity: llama-server runs from the bundled folder
echo "Verifying bundled llama-server..."
"$RES/llama/llama-server" --version 2>&1 | head -1

# ── Code signing (optional) ─────────────────────────────────────────
# Set SIGN_ID to a "Developer ID Application: NAME (TEAMID)" identity to
# sign for distribution outside the App Store. Without it, the .dmg is
# unsigned and Gatekeeper will warn.
#   SIGN_ID="Developer ID Application: Olamide Adebayo (TEAMID)"
SIGN_ID="${SIGN_ID:-}"
ENTITLEMENTS="$HERE/entitlements.plist"

if [ -n "$SIGN_ID" ]; then
  echo "Signing bundle with: $SIGN_ID"
  # Inner-to-outer, Hardened Runtime (--options runtime) + secure timestamp.
  # Every nested Mach-O must be signed or notarization fails.
  find "$RES/llama" -name "*.dylib" -print0 | while IFS= read -r -d '' lib; do
    codesign --force --timestamp --options runtime --sign "$SIGN_ID" "$lib"
  done
  codesign --force --timestamp --options runtime --sign "$SIGN_ID" "$RES/llama/llama-server"
  codesign --force --timestamp --options runtime --entitlements "$ENTITLEMENTS" --sign "$SIGN_ID" "$RES/ofin-darwin-arm64"
  # Sign the app bundle last (outermost).
  codesign --force --timestamp --options runtime --entitlements "$ENTITLEMENTS" --sign "$SIGN_ID" "$APP"
  codesign --verify --deep --strict --verbose=2 "$APP" 2>&1 | tail -2
fi

# Build .dmg with an /Applications symlink for drag-install
echo "Creating $DMG..."
rm -rf "$STAGING" "$DMG"
mkdir -p "$STAGING"
cp -R "$APP" "$STAGING/"
ln -s /Applications "$STAGING/Applications"
hdiutil create -volname "Òfin" -srcfolder "$STAGING" -ov -format UDZO "$DMG"
rm -rf "$STAGING"

# ── Notarization (optional) ─────────────────────────────────────────
# Set NOTARY_PROFILE to a keychain profile created once via:
#   xcrun notarytool store-credentials "ofin-notary" \
#       --apple-id <you@email> --team-id <TEAMID> --password <app-specific-pw>
NOTARY_PROFILE="${NOTARY_PROFILE:-}"

if [ -n "$SIGN_ID" ]; then
  codesign --force --timestamp --sign "$SIGN_ID" "$DMG"
fi
if [ -n "$NOTARY_PROFILE" ]; then
  echo "Notarizing $DMG (this can take a few minutes)..."
  xcrun notarytool submit "$DMG" --keychain-profile "$NOTARY_PROFILE" --wait
  xcrun stapler staple "$DMG"
  echo "Stapled. Verifying Gatekeeper acceptance:"
  spctl -a -t open --context context:primary-signature -vv "$DMG" 2>&1 | tail -2
fi

SIZE=$(du -h "$DMG" | cut -f1)
echo ""
echo "✓ Done: $DMG ($SIZE)"
if [ -z "$SIGN_ID" ]; then
  echo "  UNSIGNED — set SIGN_ID (and NOTARY_PROFILE) to remove the Gatekeeper warning."
elif [ -z "$NOTARY_PROFILE" ]; then
  echo "  Signed but NOT notarized — set NOTARY_PROFILE to fully clear Gatekeeper."
else
  echo "  Signed + notarized + stapled — no Gatekeeper warning."
fi
