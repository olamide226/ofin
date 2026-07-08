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

# Bundle the official llama.cpp: llama-server + every dylib it needs.
cp "$LLAMA_DIR/llama-server" "$RES/llama/"
cp "$LLAMA_DIR/"*.dylib "$RES/llama/"

chmod +x "$APP/Contents/MacOS/ofin-launcher" "$RES/ofin-darwin-arm64" "$RES/llama/llama-server"

# Sanity: llama-server runs from the bundled folder
echo "Verifying bundled llama-server..."
"$RES/llama/llama-server" --version 2>&1 | head -1

# Build .dmg with an /Applications symlink for drag-install
echo "Creating $DMG..."
rm -rf "$STAGING" "$DMG"
mkdir -p "$STAGING"
cp -R "$APP" "$STAGING/"
ln -s /Applications "$STAGING/Applications"
hdiutil create -volname "Òfin" -srcfolder "$STAGING" -ov -format UDZO "$DMG"
rm -rf "$STAGING"

SIZE=$(du -h "$DMG" | cut -f1)
echo ""
echo "✓ Done: $DMG ($SIZE)"
echo "  Standalone — bundles official llama.cpp, no external deps."
echo "  To notarize (optional):"
echo "    xcrun notarytool submit \"$DMG\" --apple-id <id> --team-id <team> --wait"
