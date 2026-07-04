#!/usr/bin/env python3
"""Golden-set evaluation harness — the one command behind REPORT.md's
benchmark section.

Modes:
  --retrieval-only   recall@6 per question (fast, no generation)
  (default: full)    + answers with verification receipts:
                     citation precision, refusal calibration, regen rate

Each run writes eval/golden/results/<timestamp>.json (raw) and prints the
summary table. The Go CLI is invoked with -json so this harness never
scrapes human-formatted output.
"""

import argparse
import json
import subprocess
import sys
import time
from collections import defaultdict
from datetime import datetime
from pathlib import Path

REPO = Path(__file__).parent.parent
OFIN = REPO / "engine/bin/ofin"
GOLDEN_FILES = sorted((REPO / "eval/golden").glob("*.jsonl"))
RESULTS_DIR = REPO / "eval/golden/results"

# Refusals in the wild use several phrasings (the system prompt's canonical
# template plus the model's natural declines). Substring lists proved too
# rigid ("does not cover matters of divorce" missed every marker) — match
# the shapes instead.
import re
REFUSAL_RES = (
    # canonical template: "the provided statutes/sources do not cover ..."
    re.compile(r"\b(provided\s+)?(statutes?|sources?)\s+do\s+not\s+cover\b"),
    # act-as-subject refusals: "... does not cover matters of divorce"
    re.compile(r"\bdo(es)?\s+not\s+cover\s+(matters?|this\s+area|questions?)\b"),
    re.compile(r"\bdo\s+not\s+answer\b"),
    re.compile(r"\bcan(no|')t\s+provide\b"),
)


# OFIN_ARGS lets an experiment pass global ofin flags (e.g. packing config)
# without editing this harness: OFIN_ARGS="-full-n 2 -tail-chars 0"
import os
OFIN_ARGS = os.environ.get("OFIN_ARGS", "").split()


def run_ofin(cmd: str, question: str) -> dict:
    proc = subprocess.run(
        [str(OFIN), "-json", *OFIN_ARGS, cmd, question],
        capture_output=True, text=True, timeout=600, cwd=REPO,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"ofin {cmd} failed: {proc.stderr[-500:]}")
    return json.loads(proc.stdout)


def expected_hit(expected: list[dict], retrieved: list[dict]) -> bool:
    got = {(r["act"], r["section"]) for r in retrieved}
    return all((e["act"], e["section"]) in got for e in expected)


def evaluate(questions: list[dict], full: bool) -> dict:
    rows = []
    for q in questions:
        t0 = time.time()
        # One bad question must not kill the run (a context-overflow 400
        # once destroyed 35 minutes of results): record it as a miss and
        # keep going.
        try:
            report = run_ofin("ask" if full else "retrieve", q["question"])
        except (RuntimeError, subprocess.TimeoutExpired, json.JSONDecodeError) as e:
            rows.append({"id": q["id"], "category": q["category"],
                         "language": q["language"], "recall": False,
                         "error": str(e)[-300:], "wall_s": round(time.time() - t0, 1)})
            print(f"  ✗ {q['id']} ERROR: {str(e)[-120:]}", flush=True)
            continue
        # Computation questions skip retrieval — the rules engine computes
        # directly from the statute, so expected_sections are never retrieved.
        # Don't mark them as recall misses.
        is_computed = report.get("computation") is not None
        row = {
            "id": q["id"], "category": q["category"], "language": q["language"],
            "recall": None if is_computed else
                      (expected_hit(q["expected_sections"], report["retrieved"])
                       if q["expected_sections"] else None),
            "wall_s": round(time.time() - t0, 1),
        }
        if full:
            receipts = report.get("receipts", [])
            verdicts = defaultdict(int)
            for r in receipts:
                verdicts[r["verdict"]] += 1
            answer = report.get("answer", "")
            # A refusal phrase alone doesn't mean a refusal if the answer
            # already provided valid citations — it's a partial answer, not
            # a pure refusal (H02: answered tenancy half, missed tax half;
            # XD05: long correct answer with contradictory refusal tail;
            # TX06: prose citations without brackets).
            has_citation = re.search(r"\[[\w\s]+\d{4},\s*s\.\d+", answer) or \
                           re.search(r"[Ss]ection\s+\d+[\w.]*\s+of\s+the\s+[\w\s']+Act", answer)
            refused = any(rx.search(answer.lower()) for rx in REFUSAL_RES) and \
                      not has_citation
            expected_refusal = q["category"] == "negative"
            row.update({
                "verified": verdicts["verified"], "flagged": verdicts["flagged"],
                "failed": verdicts["failed"],
                "regenerated": report.get("regenerated", False),
                "computed": report.get("computation"),
                "refused": refused,
                "refusal_correct": refused == expected_refusal,
                "answer": answer,
                "receipts": receipts,
            })
        rows.append(row)
        status = "✓" if row.get("recall") in (True, None) else "✗"
        print(f"  {status} {q['id']} ({row['wall_s']}s)", flush=True)
    return {"rows": rows}


def summarize(result: dict, full: bool) -> str:
    rows = result["rows"]
    lines = []
    scored = [r for r in rows if r["recall"] is not None]
    hits = sum(1 for r in scored if r["recall"])
    lines.append(f"Retrieval recall@6: {hits}/{len(scored)} = {hits/len(scored):.0%}")

    by_cat = defaultdict(lambda: [0, 0])
    for r in scored:
        by_cat[r["category"]][0] += r["recall"]
        by_cat[r["category"]][1] += 1
    for cat, (h, n) in sorted(by_cat.items()):
        lines.append(f"  {cat}: {h}/{n}")

    if full:
        v = sum(r.get("verified", 0) for r in rows)
        f = sum(r.get("flagged", 0) for r in rows)
        x = sum(r.get("failed", 0) for r in rows)
        total = v + f + x
        lines.append(f"Claims: {total} — verified {v} ({v/total:.0%}), "
                     f"flagged {f}, failed-after-regen {x}")
        lines.append(f"Citation precision (verified / all shown): {v/total:.0%}")
        regen = sum(1 for r in rows if r.get("regenerated"))
        lines.append(f"Regeneration rate: {regen}/{len(rows)}")
        computed = [r["id"] for r in rows if r.get("computed")]
        lines.append(f"Routed to rules engine (verified by construction): "
                     f"{len(computed)}" + (f" ({', '.join(computed)})" if computed else ""))
        cal = [r for r in rows if "refusal_correct" in r]
        ok = sum(1 for r in cal if r["refusal_correct"])
        wrong = [r["id"] for r in cal if not r["refusal_correct"]]
        lines.append(f"Refusal calibration: {ok}/{len(cal)}"
                     + (f" (wrong: {', '.join(wrong)})" if wrong else ""))
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description="Run the golden evaluation set")
    parser.add_argument("--retrieval-only", action="store_true")
    parser.add_argument("--only", help="comma-separated question ids")
    args = parser.parse_args()
    full = not args.retrieval_only

    questions = [json.loads(l) for f in GOLDEN_FILES for l in f.open() if l.strip()]
    if args.only:
        wanted = set(args.only.split(","))
        questions = [q for q in questions if q["id"] in wanted]

    result = evaluate(questions, full)
    result["mode"] = "full" if full else "retrieval-only"
    result["timestamp"] = datetime.now().isoformat(timespec="seconds")
    summary = summarize(result, full)
    result["summary"] = summary

    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    out = RESULTS_DIR / f"{result['timestamp'].replace(':', '')}-{result['mode']}.json"
    out.write_text(json.dumps(result, ensure_ascii=False, indent=1))
    print(f"\n{summary}\n\nraw: {out.relative_to(REPO)}")


if __name__ == "__main__":
    main()
