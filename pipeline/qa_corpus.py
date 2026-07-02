#!/usr/bin/env python3
"""Corpus/chunk quality gate for Òfin.

Validates the chunked corpus and REPORTS defects — it never mutates statute-
derived data (corpus policy: silent rewriting of legal text is worse than a
visible gap). Run after chunking; `--strict` exits non-zero on errors, which
is how `make test` wires it in.

Checks:
  E1  duplicate section_id within an act (citation ambiguity)
  E2  empty / whitespace-only chunk text
  E3  TOC-scale act: mean section text under 300 chars (body mis-detection)
  W1  suspicious title: ALL-CAPS fragment, bare "SECTION", starts lowercase,
      shorter than 4 chars, or ends mid-phrase (trailing comma/conjunction)
  W2  missing title on a plain section chunk
  W3  section-number gaps not explained by a known repeal/dropout allowlist
  W4  chunk text longer than the splitter cap should allow

Usage: qa_corpus.py [--chunks-dir data/chunks] [--strict]
"""

import argparse
import json
import re
import sys
from pathlib import Path

REPO = Path(__file__).parent.parent

# Numbering gaps with a documented explanation (see corpus/*/sources.md).
KNOWN_GAPS: dict[str, set[int]] = {
    "Trade Disputes Act 2004": set(range(20, 33)),  # repealed, NIC Act 2006
    "Employees' Compensation Act 2010": {5, 21},    # transcription dropouts, sources.md
}

SUSPICIOUS_TITLE_RE = re.compile(
    r"^(SECTION|SCHEDULE)S?$|^[A-Z\s\d.,-]{12,}$"  # bare keywords / shouty fragments
)
# "objected to", "purposes of Act" are legitimate endings — only flag
# endings that cannot close an English title.
TRAILING_FRAGMENT_RE = re.compile(r"(,|\b(and|or|of|the))$", re.IGNORECASE)


def check_act(path: Path) -> tuple[list[str], list[str]]:
    errors: list[str] = []
    warnings: list[str] = []
    data = json.loads(path.read_text(encoding="utf-8"))
    act = data["act"]
    chunks = data["chunks"]

    seen_plain_sections: set[str] = set()
    section_texts: list[str] = []
    numbers: set[int] = set()

    for c in chunks:
        m = c["metadata"]
        sid, ctype, title = m["section_id"], m["chunk_type"], m.get("section_title")
        text = c["text"]

        if not text.strip():
            errors.append(f"E2 {act} {sid}: empty chunk text")
        if ctype == "section":
            if sid in seen_plain_sections:
                errors.append(f"E1 {act} {sid}: duplicate plain-section chunk")
            seen_plain_sections.add(sid)
        if ctype.startswith("section"):
            section_texts.append(text)
            if m.get("section_number"):
                numbers.add(m["section_number"])
            if not title:
                warnings.append(f"W2 {act} {sid}: no title")
            elif (SUSPICIOUS_TITLE_RE.match(title.strip())
                  or len(title.strip()) < 4
                  or title.strip()[0].islower()
                  or TRAILING_FRAGMENT_RE.search(title.strip())):
                warnings.append(f"W1 {act} {sid}: suspicious title {title!r}")
        if len(text) > 6000:
            warnings.append(f"W4 {act} {sid}: {len(text)} chars (splitter cap not applied?)")

    if section_texts:
        mean = sum(map(len, section_texts)) / len(section_texts)
        if mean < 300:
            errors.append(f"E3 {act}: mean section text {mean:.0f} chars — TOC-chunked?")

    if numbers:
        gaps = set(range(1, max(numbers) + 1)) - numbers - KNOWN_GAPS.get(act, set())
        if gaps:
            warnings.append(f"W3 {act}: unexplained section gaps {sorted(gaps)}")

    return errors, warnings


def main() -> None:
    parser = argparse.ArgumentParser(description="Corpus chunk quality gate")
    parser.add_argument("--chunks-dir", type=Path, default=REPO / "data/chunks")
    parser.add_argument("--strict", action="store_true", help="exit 1 on errors")
    args = parser.parse_args()

    all_errors: list[str] = []
    all_warnings: list[str] = []
    for path in sorted(args.chunks_dir.glob("*.json")):
        errors, warnings = check_act(path)
        all_errors.extend(errors)
        all_warnings.extend(warnings)

    for w in all_warnings:
        print(f"  warn  {w}")
    for e in all_errors:
        print(f"  ERROR {e}")
    print(f"QA: {len(all_errors)} errors, {len(all_warnings)} warnings")
    if args.strict and all_errors:
        sys.exit(1)


if __name__ == "__main__":
    main()
