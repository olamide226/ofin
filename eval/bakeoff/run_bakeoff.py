#!/usr/bin/env python3
"""Week 1 model bake-off runner.

Runs every question in questions.jsonl against each candidate GGUF via
llama-cli (single-turn chat, model's own chat template), captures the reply
and speed stats, and writes:

  outputs/<model-tag>/<question-id>.txt   — the raw reply
  results.csv                             — model, question, tps, latency

Scoring stays manual (see RUBRIC.md) — 60 short legal answers deserve human
eyes; automation here is only for the mechanical part.
"""

import csv
import json
import re
import subprocess
import sys
import time
from pathlib import Path

HERE = Path(__file__).parent
MODELS_DIR = HERE / "../../models-dev"
MODELS = {
    "qwen2.5-3b": "Qwen2.5-3B-Instruct-Q4_K_M.gguf",
    "llama3.2-3b": "Llama-3.2-3B-Instruct-Q4_K_M.gguf",
    "phi3.5-mini": "Phi-3.5-mini-instruct-Q4_K_M.gguf",
}
GEN_TOKENS = 600
TEMP = 0.2

STATS_RE = re.compile(r"\[ Prompt: ([\d.]+) t/s \| Generation: ([\d.]+) t/s \]")


def extract_reply(stdout: str, prompt: str) -> str:
    """The reply sits between the echoed '> <prompt>' line and the stats line."""
    first_line = prompt.splitlines()[0][:40]
    lines = stdout.splitlines()
    start = 0
    for i, line in enumerate(lines):
        if line.startswith("> ") and first_line[:20] in line:
            start = i + 1
            break
    reply: list[str] = []
    for line in lines[start:]:
        if STATS_RE.search(line) or line.startswith("Exiting"):
            break
        reply.append(line)
    text = "\n".join(reply).strip()
    return re.sub(r"^[|\-\\/\s]+", "", text)  # strip loading-spinner chars


def main() -> None:
    questions = [json.loads(l) for l in (HERE / "questions.jsonl").open() if l.strip()]
    only_model = sys.argv[1] if len(sys.argv) > 1 else None
    rows = []

    for tag, fname in MODELS.items():
        if only_model and tag != only_model:
            continue
        model_path = MODELS_DIR / fname
        outdir = HERE / "outputs" / tag
        outdir.mkdir(parents=True, exist_ok=True)

        for q in questions:
            t0 = time.time()
            proc = subprocess.run(
                ["llama-cli", "-m", str(model_path),
                 "-sys", q["system"], "-p", q["prompt"],
                 "-st", "-n", str(GEN_TOKENS), "--temp", str(TEMP)],
                capture_output=True, text=True, timeout=600,
            )
            wall = time.time() - t0
            reply = extract_reply(proc.stdout, q["prompt"])
            stats = STATS_RE.search(proc.stdout)
            prompt_tps, gen_tps = (stats.group(1), stats.group(2)) if stats else ("", "")

            (outdir / f"{q['id']}.txt").write_text(reply + "\n")
            rows.append({"model": tag, "question": q["id"], "category": q["category"],
                         "wall_s": f"{wall:.1f}", "prompt_tps": prompt_tps,
                         "gen_tps": gen_tps, "reply_chars": len(reply)})
            print(f"{tag} {q['id']}: {wall:.1f}s gen={gen_tps} t/s "
                  f"{len(reply)} chars", flush=True)

    results = HERE / "results.csv"
    exists = results.exists()
    with results.open("a", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=list(rows[0].keys()))
        if not exists:
            writer.writeheader()
        writer.writerows(rows)
    print(f"\nWrote {len(rows)} rows to {results}")


if __name__ == "__main__":
    main()
