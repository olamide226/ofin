#!/usr/bin/env python3
"""Strip gazette margin notes from the NTA 2025 corpus.

The Nigeria Tax Act 2025 PDF was extracted with pdftotext -layout, which
preserves the bimodal shoulder-note layout of the official gazette:
  - Right-margin notes: body text followed by 6+ spaces then a short note
    (e.g. "Rates of tax for individuals", "Act No. 8, 2019")
  - Continuation lines: very short lines with large leading whitespace
    that continue a shoulder note from the previous line

This script strips both while preserving body text. It writes to a
separate output file so the original is preserved for comparison.

Conservative: only acts on lines matching the right-margin pattern
(body + 6+ spaces + short tail). Continuation lines are only removed
if they follow a right-margin line and are short + heavily indented.
"""

import re
import sys
from pathlib import Path

REPO = Path(__file__).parent.parent

# Right-margin note: body text (55+ chars) followed by 6+ spaces then
# a short bit of text (1-50 chars) — the shoulder note.
RIGHT_MARGIN = re.compile(r"^(.{55,}?)\s{6,}(\S.{0,50})$")

# Continuation of a margin note: line that is mostly whitespace
# (30+ leading spaces) and very short text (1-45 chars).
MARGIN_CONTINUATION = re.compile(r"^\s{30,}(\S.{0,45})$")

# Lines that are exclusively margin notes (no body text at all):
# large leading whitespace, tiny amount of text at the right edge.
# These are safe to remove — they're orphaned continuation lines.


def clean_nta(input_path: Path, output_path: Path) -> int:
    """Clean the NTA corpus file. Returns number of lines modified."""
    with open(input_path) as f:
        lines = f.readlines()

    cleaned = []
    changes = 0
    prev_was_margin = False

    for i, line in enumerate(lines):
        stripped = line.rstrip("\n")
        m = RIGHT_MARGIN.match(stripped)

        if m:
            # Keep only the body text
            body = m.group(1).rstrip()
            note = m.group(2).strip()
            cleaned.append(body + "\n")
            changes += 1
            prev_was_margin = True
        elif prev_was_margin and MARGIN_CONTINUATION.match(stripped):
            # This is a continuation of the shoulder note — skip it
            changes += 1
            # prev_was_margin stays True for multi-line continuations
        else:
            cleaned.append(line)
            prev_was_margin = False

    with open(output_path, "w") as f:
        f.writelines(cleaned)

    return changes


def main():
    nta = REPO / "corpus" / "tax" / "nigeria-tax-act-2025.md"
    out = REPO / "corpus" / "tax" / "nigeria-tax-act-2025-cleaned.md"

    changes = clean_nta(nta, out)

    print(f"Lines cleaned: {changes}")
    print(f"Original: {nta} ({nta.stat().st_size:,} bytes)")
    print(f"Cleaned:  {out} ({out.stat().st_size:,} bytes)")
    print(f"Diff:     {out.stat().st_size - nta.stat().st_size:+,} bytes")

    # Show a few examples of what changed
    print("\n--- Sample changes (first 5) ---")
    with open(nta) as f:
        old_lines = f.readlines()
    with open(out) as f:
        new_lines = f.readlines()

    shown = 0
    for i, (old, new) in enumerate(zip(old_lines, new_lines)):
        if old != new and shown < 5:
            print(f"\nL{i+1} BEFORE: {old.rstrip()[:120]}")
            print(f"L{i+1} AFTER:  {new.rstrip()[:120]}")
            shown += 1


if __name__ == "__main__":
    main()
