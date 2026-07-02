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
GOLDEN = REPO / "eval/golden/labour.jsonl"
RESULTS_DIR = REPO / "eval/golden/results"

REFUSAL_MARKER = "do not answer"


def run_ofin(cmd: str, question: str) -> dict:
    proc = subprocess.run(
        [str(OFIN), "-json", cmd, question],
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
        report = run_ofin("ask" if full else "retrieve", q["question"])
        row = {
            "id": q["id"], "category": q["category"], "language": q["language"],
            "recall": expected_hit(q["expected_sections"], report["retrieved"])
                      if q["expected_sections"] else None,
            "wall_s": round(time.time() - t0, 1),
        }
        if full:
            receipts = report.get("receipts", [])
            verdicts = defaultdict(int)
            for r in receipts:
                verdicts[r["verdict"]] += 1
            answer = report.get("answer", "")
            refused = REFUSAL_MARKER in answer.lower()
            expected_refusal = q["category"] == "negative"
            row.update({
                "verified": verdicts["verified"], "flagged": verdicts["flagged"],
                "failed": verdicts["failed"],
                "regenerated": report.get("regenerated", False),
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
        v = sum(r["verified"] for r in rows)
        f = sum(r["flagged"] for r in rows)
        x = sum(r["failed"] for r in rows)
        total = v + f + x
        lines.append(f"Claims: {total} — verified {v} ({v/total:.0%}), "
                     f"flagged {f}, failed-after-regen {x}")
        lines.append(f"Citation precision (verified / all shown): {v/total:.0%}")
        regen = sum(1 for r in rows if r.get("regenerated"))
        lines.append(f"Regeneration rate: {regen}/{len(rows)}")
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

    questions = [json.loads(l) for l in GOLDEN.open() if l.strip()]
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
