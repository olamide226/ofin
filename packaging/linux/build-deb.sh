#!/bin/bash
# Build a standalone .deb package for Òfin.
# Usage: bash build-deb.sh <version>
#
# Bundles the official llama.cpp build (binary + .so libraries) under
# /usr/lib/ofin/llama/. Only the 1.9 GB model downloads on first launch.
#
# Prerequisites (env or edit below):
#   OFIN_BIN     path to ofin-linux-x86_64
#   LLAMA_DIR    path to extracted official llama.cpp ubuntu release folder
#   EMBED_MODEL  path to bge-small-en-v1.5-f16.gguf
#   OFIN_DB      path to data/ofin.db

set -euo pipefail
VERSION="${1:-0.2.0}"
HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"

OFIN_BIN="${OFIN_BIN:-$REPO/engine/bin/ofin-linux-x86_64}"
LLAMA_DIR="${LLAMA_DIR:-$HOME/llama.cpp-linux}"
EMBED_MODEL="${EMBED_MODEL:-$REPO/models-dev/bge-small-en-v1.5-f16.gguf}"
OFIN_DB="${OFIN_DB:-$REPO/data/ofin.db}"
ICON="$HERE/../icons/ofin.png"

STAGING="$HERE/ofin_${VERSION}_amd64"
DEB="$HERE/ofin_${VERSION}_amd64.deb"

echo "=== Òfin Linux .deb Builder (standalone) ==="

for f in "$OFIN_BIN" "$EMBED_MODEL" "$OFIN_DB" "$ICON"; do
  [ -f "$f" ] || { echo "ERROR: missing $f"; exit 1; }
done
[ -x "$LLAMA_DIR/llama-server" ] || { echo "ERROR: llama-server not in $LLAMA_DIR"; exit 1; }

rm -rf "$STAGING" "$DEB"

# ---- Layout ----
#   /usr/lib/ofin/ofin              Go binary
#   /usr/lib/ofin/llama/            official llama.cpp (server + .so)
#   /usr/bin/ofin-launch            wrapper
#   /usr/share/ofin/                corpus DB + embed model (seed copies)
#   /usr/share/applications/        desktop entry
#   /usr/share/icons/...            app icon
mkdir -p "$STAGING/DEBIAN" \
         "$STAGING/usr/lib/ofin/llama" \
         "$STAGING/usr/bin" \
         "$STAGING/usr/share/ofin" \
         "$STAGING/usr/share/applications" \
         "$STAGING/usr/share/icons/hicolor/256x256/apps"

cp "$OFIN_BIN" "$STAGING/usr/lib/ofin/ofin"
cp "$LLAMA_DIR/llama-server" "$STAGING/usr/lib/ofin/llama/"
cp "$LLAMA_DIR/"*.so* "$STAGING/usr/lib/ofin/llama/" 2>/dev/null || true
cp "$OFIN_DB" "$STAGING/usr/share/ofin/ofin.db"
cp "$EMBED_MODEL" "$STAGING/usr/share/ofin/bge-small-en-v1.5-f16.gguf"
echo "$VERSION" > "$STAGING/usr/share/ofin/VERSION"
cp "$ICON" "$STAGING/usr/share/icons/hicolor/256x256/apps/ofin.png"
cp "$HERE/usr/share/applications/ofin.desktop" "$STAGING/usr/share/applications/ofin.desktop"

# Launcher wrapper — ofin binary finds llama-server in ./llama/ itself.
cat > "$STAGING/usr/bin/ofin-launch" << 'LAUNCHER'
#!/bin/bash
DATA="$HOME/.local/share/ofin"
# Seed (or re-seed on upgrade) the corpus DB + embedding model. Gated on a
# version stamp, not just "does ofin.db exist" — otherwise a corpus fix
# shipped in a later version would never reach an already-installed user.
# The 1.9 GB chat model is untouched here: it's fetched once, separately,
# and doesn't change between versions.
mkdir -p "$DATA/data" "$DATA/model" "$DATA/models-dev"
BUNDLED_VERSION="$(cat /usr/share/ofin/VERSION 2>/dev/null || true)"
INSTALLED_VERSION="$(cat "$DATA/.version" 2>/dev/null || true)"
if [ "$BUNDLED_VERSION" != "$INSTALLED_VERSION" ] || [ ! -f "$DATA/data/ofin.db" ]; then
  cp /usr/share/ofin/ofin.db "$DATA/data/ofin.db"
  cp /usr/share/ofin/bge-small-en-v1.5-f16.gguf "$DATA/models-dev/bge-small-en-v1.5-f16.gguf"
  echo "$BUNDLED_VERSION" > "$DATA/.version"
fi
# Start Òfin. Web server shows a download page if the model is missing.
/usr/lib/ofin/ofin serve --data-dir "$DATA" &
sleep 3
xdg-open "http://127.0.0.1:8090" 2>/dev/null || true
wait
LAUNCHER
chmod +x "$STAGING/usr/bin/ofin-launch"

# Control + postinst
cp "$HERE/DEBIAN/control" "$STAGING/DEBIAN/control"
sed -i "s/^Version:.*/Version: $VERSION/" "$STAGING/DEBIAN/control"
cp "$HERE/DEBIAN/postinst" "$STAGING/DEBIAN/postinst"
chmod 0755 "$STAGING/DEBIAN/postinst"

# Build
dpkg-deb --build --root-owner-group "$STAGING" "$DEB"
rm -rf "$STAGING"

SIZE=$(du -h "$DEB" | cut -f1)
echo "✓ Done: $DEB ($SIZE)"
