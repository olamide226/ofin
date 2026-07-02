#!/usr/bin/env bash
# Benchmark: N timed ofin ask runs (mixed lookup + computation).
# Reports total wall time and peak RSS of the llama-server processes.
# Usage: bash scripts/bench_ask.sh [runs=3] [--with-draft]
set -euo pipefail

RUNS=${1:-3}
DRAFT_FLAG=""   # draft is off by default (ADR-012)
if [[ "${2:-}" == "--with-draft" ]]; then
  DRAFT_FLAG="--draft"
fi

cd "$(dirname "$0")/.."
BIN=engine/bin/ofin

# Questions covering both lookup and computation paths.
QUESTIONS=(
  "I have worked at my company for 3 years and they want to sack me. How much notice must they give me?"
  "I earn 450,000 naira monthly. How much PAYE tax should I be paying?"
  "What maternity leave and pay is a pregnant employee entitled to under Nigerian law?"
)

echo "# ofin bench — $(date '+%Y-%m-%d %H:%M')"
echo "# runs=$RUNS draft=${DRAFT_FLAG:-no}"
echo "# model: $($BIN ask --help 2>/dev/null | head -1 || echo 'ofin')"
echo "#"
printf "%-6s %-8s %-8s %-10s %-10s %-8s\n" \
  "run" "q" "kind" "wall_s" "rss_server" "rss_embed"
echo "------ ------  ------  ----------  ----------  --------"

for i in $(seq 1 $RUNS); do
  qi=$(( (i-1) % ${#QUESTIONS[@]} ))
  q="${QUESTIONS[$qi]}"

  t0=$SECONDS
  out=$($BIN $DRAFT_FLAG ask "$q" 2>&1)
  wall=$(( SECONDS - t0 ))

  # Classify the path taken
  if echo "$out" | grep -q "routed to computation"; then
    kind="compute"
  else
    kind="lookup"
  fi

  # Peak RSS of llama-server processes
  chat_rss=$(ps -o rss= -p $(pgrep -f "llama-server.*8092" | head -1) 2>/dev/null || echo 0)
  embed_rss=$(ps -o rss= -p $(pgrep -f "llama-server.*8091" | head -1) 2>/dev/null || echo 0)
  # macOS ps reports RSS in KiB; convert to MB
  chat_mb=$(( chat_rss / 1024 ))
  embed_mb=$(( embed_rss / 1024 ))

  printf "%-6s %-8s %-8s %-10s %-10s %-8s\n" \
    "$i" "${q:0:8}" "$kind" "$wall" "${chat_mb} MB" "${embed_mb} MB"
done

echo ""
echo "# Total stack RSS: chat server + embed server + ofin (transient)"
echo "# Dev machine: $(sysctl -n hw.model 2>/dev/null || uname -m), $(sysctl -n hw.memsize 2>/dev/null | awk '{printf "%d GB", $1/1073741824}' || echo "? GB")"
