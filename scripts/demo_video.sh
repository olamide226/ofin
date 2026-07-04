#!/usr/bin/env bash
# Records the Òfin demo video.
# Prerequisites: ofin server running, Chrome installed, node available.
set -euo pipefail

OUT="${1:-docs/screenshots/demo-video.mp4}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"

echo "=== Òfin Demo Video ==="
echo "Output: $OUT"
echo ""

# 1. Start the server if not already running
if ! curl -s -o /dev/null http://127.0.0.1:8090/ 2>/dev/null; then
  echo "Starting Òfin server..."
  "$REPO/engine/bin/ofin" serve --port 8090 &
  sleep 5
fi

echo "Server ready. Recording will start in 3 seconds..."
echo "DO NOT move the Chrome window — it will appear at top-left."
sleep 3

# 2. Start screen recording in background (130 seconds = 2 min 10 sec)
#   -v: video mode
#   -V 130: limit to 130 seconds
#   -R: region to record (x, y, width, height)
#   Chrome opens at (200,100) with size 1280x800
#   Menu bar is ~24px, so y=124
echo "Recording started (130 sec)..."
screencapture -v -V 140 -R 200,100,1280,830 "$REPO/$OUT" &
RECORD_PID=$!
sleep 2

# 3. Run the demo interactions in visible Chrome
echo "Running demo script..."
node "$REPO/scripts/record_demo.js" 2>&1
echo "Demo interactions complete."

# 4. Wait for recording to finish
echo "Waiting for recording to finish..."
wait $RECORD_PID 2>/dev/null || true

# 5. Verify
if [ -f "$REPO/$OUT" ]; then
  SIZE=$(du -h "$REPO/$OUT" | cut -f1)
  echo ""
  echo "✓ Demo video saved: $OUT ($SIZE)"
  echo "  Open with: open $OUT"
else
  echo "✗ Recording failed — no output file."
  exit 1
fi
