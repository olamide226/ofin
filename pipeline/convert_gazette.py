#!/usr/bin/env python3
"""Convert pdftotext -layout gazette extractions to corpus markdown.

Strips the recurring page furniture (running headers like
"Nigeria Tax Act, 2025   2025 No. 7   A 399", bare page markers, form
feeds) while leaving statutory text untouched — corpus policy: never
silently alter legal text, only remove layout artifacts.

Usage: convert_gazette.py IN.txt OUT.md "Title for the # heading"
"""

import re
import sys
from pathlib import Path

HEADER_PATTERNS = [
    re.compile(r"^\s*[A-Z][\w\s,()']*Act,?\s*20\d\d\s+20\d\d\s+No\.\s*\d+\s+A\s*\d+\s*$"),
    re.compile(r"^\s*A\s*\d{1,4}\s+20\d\d\s+No\.\s*\d+\s+[A-Z][\w\s,()']*20\d\d\s*$"),
    re.compile(r"^\s*(A\s*\d{1,4}|\d{1,4})\s*$"),          # bare page numbers
    re.compile(r"^\s*Lagos State of Nigeria\s*$"),
    re.compile(r"^\s*Official Gazette\s*$"),
    re.compile(r"^\s*Tenancy Law 2011\s*$"),
]


def convert(src: Path, dst: Path, title: str) -> None:
    text = src.read_text(encoding="utf-8", errors="replace")
    out_lines: list[str] = []
    for page in text.split("\f"):
        for line in page.splitlines():
            if any(p.match(line) for p in HEADER_PATTERNS):
                continue
            out_lines.append(line.rstrip())
    body = "\n".join(out_lines)
    body = re.sub(r"\n{3,}", "\n\n", body)
    dst.write_text(f"# {title}\n\n{body.strip()}\n", encoding="utf-8")
    print(f"{dst}: {len(body)} chars")


if __name__ == "__main__":
    convert(Path(sys.argv[1]), Path(sys.argv[2]), sys.argv[3])
