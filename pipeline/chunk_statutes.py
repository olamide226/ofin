#!/usr/bin/env python3
"""Statute-aware chunker for the Òfin corpus.

Parses cleaned statute markdown (corpus/*/[act].md) into section-level chunk
JSON files compatible with the SAC enrichment stage
(pipeline/enhance_chunks_with_sac.py, ported from Immigranta).

Design:
- One chunk per statutory section — the section is both the retrieval unit
  and the citation unit the verifier checks against.
- Section titles come from the Arrangement of Sections (TOC): in the LFN
  print layout, body titles live in margin notes that do not survive
  extraction, so the TOC is the only reliable title source.
- Body section starts are matched by number + content and accepted only in
  ascending sequence, so numbered schedule paragraphs and TOC entries cannot
  masquerade as sections.
- Sections longer than MAX_SECTION_CHARS split at subsection boundaries.

Usage:
    python3 pipeline/chunk_statutes.py [--domain labour] [--out data/chunks]
"""

import argparse
import json
import re
from dataclasses import dataclass, field
from datetime import date
from pathlib import Path

REPO = Path(__file__).parent.parent
MAX_SECTION_CHARS = 4000

# Section start in body text: "11. (1) Either party…", "54.(1) In any…",
# "3.—(1) Every employer…", "15. Wages shall become due…"
BODY_SECTION_RE = re.compile(r"^\s{0,8}(\d{1,3})\s*\.\s*(?:[—–-]\s*)?(\(1\)|[A-Z(\"'])")
# Part heading: "### Part II", "### PART III COMPENSATION…", "PART V …"
PART_RE = re.compile(r"^#{0,4}\s*PART\s+([IVXLC\d]+)\b[.\s]*(.*)$", re.IGNORECASE)
SUBSECTION_RE = re.compile(r"^\(\d{1,2}\)")
# Schedule heading: "SCHEDULE", "FIRST SCHEDULE", "SCHEDULES", "Schedule"
SCHEDULE_RE = re.compile(r"^\s*(?:(FIRST|SECOND|THIRD|FOURTH|FIFTH)\s+)?SCHEDULES?\s*$", re.IGNORECASE)


@dataclass
class ActConfig:
    file: str                 # path relative to repo root
    act_short: str            # citation-token name, e.g. "Labour Act 2004"
    source: str               # full formal name
    citation: str             # gazette/LFN reference
    jurisdiction: str = "federal"
    notes: str = ""


LABOUR_ACTS = [
    ActConfig(
        file="corpus/labour/labour-act-cap-l1-lfn-2004.md",
        act_short="Labour Act 2004",
        source="Labour Act, Cap L1 LFN 2004",
        citation="Cap L1, Laws of the Federation of Nigeria 2004",
    ),
    ActConfig(
        file="corpus/labour/employees-compensation-act-2010.md",
        act_short="Employees' Compensation Act 2010",
        source="Employees' Compensation Act, 2010",
        citation="Act No. 13 of 2010",
    ),
    ActConfig(
        file="corpus/labour/trade-disputes-act-cap-t8-lfn-2004.md",
        act_short="Trade Disputes Act 2004",
        source="Trade Disputes Act, Cap T8 LFN 2004",
        citation="Cap T8, Laws of the Federation of Nigeria 2004",
    ),
    ActConfig(
        file="corpus/labour/trade-disputes-essential-services-act-cap-t9-lfn-2004.md",
        act_short="Trade Disputes (Essential Services) Act 2004",
        source="Trade Disputes (Essential Services) Act, Cap T9 LFN 2004",
        citation="Cap T9, Laws of the Federation of Nigeria 2004",
    ),
    ActConfig(
        file="corpus/labour/national-minimum-wage-act-2019-consolidated.md",
        act_short="NMW Act 2019",
        source="National Minimum Wage Act 2019 (as amended 2024)",
        citation="Act No. 8 of 2019, amended 2024",
    ),
]

DOMAINS = {"labour": LABOUR_ACTS}


def parse_toc_titles(lines: list[str], body_start: int) -> dict[int, str]:
    """Extract {section_number: title} from the Arrangement of Sections.

    Handles both single-line entries ("11. Termination of contracts by
    notice.") and the split layout ("11." on its own line, title on the next
    non-empty line). Only text before the body is considered.
    """
    titles: dict[int, str] = {}
    i = 0
    while i < body_start:
        line = lines[i].strip()
        m = re.match(r"^(\d{1,3})\s*\.\s*(.*)$", line)
        if m:
            num = int(m.group(1))
            title = m.group(2).strip().rstrip(".")
            if not title:
                j = i + 1
                while j < body_start and not lines[j].strip():
                    j += 1
                if j < body_start and not re.match(r"^\d{1,3}\s*\.", lines[j].strip()):
                    title = lines[j].strip().rstrip(".")
                    i = j
            # First occurrence wins; sane titles only.
            if num not in titles and title and len(title) < 150:
                titles[num] = title
        i += 1
    return titles


def _normalized(line: str) -> str:
    """Markdown headings ('## 3. Employer to pay…') carry section starts in
    hand-consolidated files — strip the marks before matching."""
    return re.sub(r"^#{1,4}\s*", "", line)


def _section_candidates(lines: list[str]) -> list[tuple[int, int]]:
    """All (line_index, section_number) lines that could start a section."""
    out = []
    for i, line in enumerate(lines):
        m = BODY_SECTION_RE.match(_normalized(line))
        if m:
            out.append((i, int(m.group(1))))
    return out


def find_body_start(lines: list[str]) -> int:
    """Locate where the enacted body begins.

    Both the Arrangement of Sections and the body contain an ascending run
    of section numbers starting at 1, and the TOC always comes first — so
    walk every maximal ascending run (±1 tolerance for transcription
    dropouts) among candidate lines and take the start of the LAST longest
    run. Falls back to 0 for files with no TOC.
    """
    candidates = _section_candidates(lines)
    if not candidates:
        return 0
    runs: list[tuple[int, int]] = []  # (start_line, run_length)
    i = 0
    while i < len(candidates):
        line_idx, num = candidates[i]
        if num != 1:
            i += 1
            continue
        length = 1
        expected = 2
        j = i + 1
        while j < len(candidates):
            _, n = candidates[j]
            if n in (expected, expected + 1):
                length += 1
                expected = n + 1
                j += 1
            elif n < expected:
                j += 1  # stray lower number inside a section body
            else:
                break
        runs.append((line_idx, length))
        # Advance by ONE, not to the walk's end: a completed walk consumes
        # later lower-numbered candidates as strays, which would hide a
        # subsequent legitimate run (e.g. NMW body headings after its TOC).
        i += 1
    if not runs:
        return candidates[0][0]
    best_len = max(length for _, length in runs)
    # Last run with (near-)best length wins: TOC and body runs are similar
    # length, and the body is always the later one.
    for start, length in reversed(runs):
        if length >= best_len - 2:
            return start
    return runs[-1][0]


@dataclass
class Section:
    number: int
    lines: list[str] = field(default_factory=list)
    part: str = ""
    title_hint: str = ""


def parse_sections(lines: list[str], body_start: int) -> tuple[str, list[Section], list[Section]]:
    """Split body into preamble + ascending-numbered sections.

    A numbered line is accepted as a new section only if its number is
    last+1 (±1 tolerance for extraction dropouts); anything else — schedule
    paragraphs, cross-reference numbers — stays inside the current section.
    """
    preamble = "\n".join(lines[:body_start]).strip()
    sections: list[Section] = []
    schedules: list[Section] = []  # number = ordinal; separate identity space
    current: Section | None = None
    current_part = ""
    pending_title = ""  # LFN prints put the marginal title on the line above
    in_schedules = False

    for line in lines[body_start:]:
        # Schedules follow the sections; each SCHEDULE heading starts a new
        # block with its own identity (never attributed to the last section).
        if SCHEDULE_RE.match(_normalized(line).strip()):
            if current is not None and not in_schedules:
                sections.append(current)
            in_schedules = True
            current = Section(number=len(schedules) + 1, part="")
            schedules.append(current)
            current.lines.append(_normalized(line).strip())
            continue
        if in_schedules:
            if line.strip():
                current.lines.append(line.rstrip())
                if not current.title_hint and len(line.strip()) < 90:
                    current.title_hint = line.strip().rstrip(".")
            continue
        norm = _normalized(line)
        pm = PART_RE.match(norm.strip())
        if pm:
            label = f"Part {pm.group(1)}"
            if pm.group(2).strip():
                label += f" — {pm.group(2).strip().title()}"
            current_part = label
            pending_title = ""
            continue

        m = BODY_SECTION_RE.match(norm)
        if m:
            num = int(m.group(1))
            expected = current.number + 1 if current else 1
            # Repealed ranges leave legitimate numbering gaps (e.g. Trade
            # Disputes Act ss.20-32 repealed by the NIC Act 2006) — accept a
            # large jump when the text since the last section says so.
            repeal_jump = (
                current is not None
                and num > expected + 1
                and any("repealed" in l.lower() for l in current.lines[-6:])
            )
            if current is None or num in (expected, expected + 1) or repeal_jump:
                if current is not None:
                    sections.append(current)
                current = Section(number=num, part=current_part, title_hint=pending_title)
                current.lines.append(norm.rstrip())
                pending_title = ""
                continue

        stripped = norm.strip()
        # A short, unnumbered, prose-free line is likely the marginal title
        # of the NEXT section (ECA-2010 layout).
        if stripped and len(stripped) < 90 and not stripped.startswith("(") \
                and not stripped[0].isdigit() and not stripped.endswith((";", ",", "-", "—")):
            pending_title = stripped.rstrip(".")
        elif stripped:
            pending_title = ""

        if current is not None:
            current.lines.append(line.rstrip())

    if current is not None and not in_schedules:
        sections.append(current)
    return preamble, sections, schedules


def split_long_section(text: str) -> list[str]:
    """Split at subsection boundaries, packing greedily under the cap."""
    if len(text) <= MAX_SECTION_CHARS:
        return [text]
    paragraphs = text.split("\n\n")
    parts: list[str] = []
    buf: list[str] = []
    size = 0
    for para in paragraphs:
        if buf and size + len(para) > MAX_SECTION_CHARS and SUBSECTION_RE.match(para.strip()):
            parts.append("\n\n".join(buf))
            buf, size = [], 0
        buf.append(para)
        size += len(para) + 2
    if buf:
        parts.append("\n\n".join(buf))
    return parts


def chunk_act(cfg: ActConfig, as_at: str) -> dict:
    raw = (REPO / cfg.file).read_text(encoding="utf-8")
    lines = raw.splitlines()
    body_start = find_body_start(lines)
    titles = parse_toc_titles(lines, body_start)
    preamble, sections, schedules = parse_sections(lines, body_start)

    base_meta = {
        "source": cfg.source,
        "act_short": cfg.act_short,
        "citation": cfg.citation,
        "jurisdiction": cfg.jurisdiction,
        "as_at": as_at,
        "file": cfg.file,
    }

    chunks: list[dict] = []
    if preamble and len(preamble) > 200:
        chunks.append({
            "metadata": {**base_meta, "part": "", "section_id": "preamble",
                         "section_number": None,
                         "section_title": "Long title, commencement and arrangement of sections",
                         "chunk_type": "preamble"},
            "text": preamble,
        })

    for sec in sections:
        text = "\n".join(sec.lines).strip()
        # Title priority: TOC entry → marginal line above (ECA layout) →
        # inline heading "N. Title" when the first line is short and the
        # substance starts at "(1)" (T8 layout).
        title = titles.get(sec.number) or sec.title_hint or None
        if not title and sec.lines:
            first = sec.lines[0].strip()
            m = re.match(rf"^{sec.number}\s*\.\s*(.+)$", first)
            if m and len(m.group(1)) < 90 and "(1)" not in m.group(1):
                title = m.group(1).strip().rstrip(".")

        pieces = split_long_section(text)
        for idx, piece in enumerate(pieces):
            section_id = f"s.{sec.number}"
            chunk_type = "section" if len(pieces) == 1 else f"section-part-{idx + 1}-of-{len(pieces)}"
            chunks.append({
                "metadata": {**base_meta, "part": sec.part, "section_id": section_id,
                             "section_number": sec.number,
                             "section_title": title,
                             "chunk_type": chunk_type},
                "text": piece,
            })

    for sch in schedules:
        text = "\n".join(sch.lines).strip()
        if len(text) < 80:  # bare "SCHEDULES" divider with no content
            continue
        pieces = split_long_section(text)
        for idx, piece in enumerate(pieces):
            chunk_type = "schedule" if len(pieces) == 1 else f"schedule-part-{idx + 1}-of-{len(pieces)}"
            chunks.append({
                "metadata": {**base_meta, "part": "", "section_id": f"sch.{sch.number}",
                             "section_number": None,
                             "section_title": sch.title_hint or f"Schedule {sch.number}",
                             "chunk_type": chunk_type},
                "text": piece,
            })

    return {
        "act": cfg.act_short,
        "source_file": cfg.file,
        "toc_titles_found": len(titles),
        "sections_found": len(sections),
        "last_section_number": sections[-1].number if sections else 0,
        "chunk_count": len(chunks),
        "chunks": chunks,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Chunk statute markdown into section-level JSON")
    parser.add_argument("--domain", default="labour", choices=sorted(DOMAINS))
    parser.add_argument("--out", type=Path, default=REPO / "data/chunks")
    args = parser.parse_args()

    args.out.mkdir(parents=True, exist_ok=True)
    as_at = date.today().strftime("%Y-%m")

    for cfg in DOMAINS[args.domain]:
        result = chunk_act(cfg, as_at)
        slug = Path(cfg.file).stem
        out_path = args.out / f"{slug}.json"
        out_path.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        titled = sum(1 for c in result["chunks"] if c["metadata"].get("section_title"))
        print(f"{cfg.act_short}: {result['sections_found']} sections "
              f"(last=s.{result['last_section_number']}, TOC titles={result['toc_titles_found']}, "
              f"titled chunks={titled}/{result['chunk_count']}) -> {out_path.relative_to(REPO)}")


if __name__ == "__main__":
    main()
