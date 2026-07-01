#!/usr/bin/env bash
# ADTC 2026 submission — downloads the Òfin model weights to model/.
# Idempotent: skips the download when a complete file is already present.
# Requires no credentials (public Hugging Face repo).
set -euo pipefail

# Base model locked 2026-07-01 by bake-off (docs/DECISIONS.md ADR-006).
# Final submission may point to a Ruach Tech HF repo with the system prompt
# baked into the chat template (Week 7 packaging).
MODEL_URL="https://huggingface.co/bartowski/Llama-3.2-3B-Instruct-GGUF/resolve/main/Llama-3.2-3B-Instruct-Q4_K_M.gguf"
MODEL_PATH="model/ofin-model.gguf"
EXPECTED_MIN_BYTES=1500000000  # sanity floor: a 3B Q4_K_M is ~2 GB

mkdir -p model

if [ -f "$MODEL_PATH" ]; then
  size=$(wc -c < "$MODEL_PATH" | tr -d ' ')
  if [ "$size" -ge "$EXPECTED_MIN_BYTES" ]; then
    echo "Model already present at $MODEL_PATH ($size bytes) — skipping download."
    exit 0
  fi
  echo "Partial file found ($size bytes) — resuming download."
fi

curl -L -C - --fail --retry 3 -o "$MODEL_PATH" "$MODEL_URL"

size=$(wc -c < "$MODEL_PATH" | tr -d ' ')
if [ "$size" -lt "$EXPECTED_MIN_BYTES" ]; then
  echo "ERROR: downloaded file is smaller than expected ($size bytes)." >&2
  exit 1
fi

echo "Model downloaded to $MODEL_PATH ($size bytes)."
