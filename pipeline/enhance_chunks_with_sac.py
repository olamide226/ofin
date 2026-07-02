#!/usr/bin/env python3
"""Summary-Augmented Chunking (SAC) enrichment for the Òfin statute corpus.

Ported from Immigranta (data_pipeline/processing/enhance_chunks_with_sac.py)
and adapted for Nigerian statutory text:

- The prompt targets statute sections (who it covers, the rule, and — most
  importantly — conditions, exceptions and thresholds: the Week-1 bake-off
  showed provisos are what small models miss).
- Each call also extracts `cross_refs`: citations to other sections or acts
  mentioned in the chunk. These become the cross-domain retrieval edges in
  Week 4 without a second enrichment pass.
- No LLM metadata enrichment: the statute chunker supplies section identity
  deterministically.
- Default model is Gemini 2.5 Flash-Lite (build-time cloud use is allowed by
  the challenge; runtime stays 100% offline).

Reads chunk JSON from data/chunks/, writes enriched copies with `summary`,
`cross_refs`, and `augmented_text` (the embedded representation) to
data/chunks-sac/. Resume is per-chunk; safe to re-run.

Usage:
    python3 pipeline/enhance_chunks_with_sac.py [--limit 1] [--force]
"""

import argparse
import asyncio
import json
import logging
import os
from pathlib import Path
from typing import Any, Optional

import dotenv
from pydantic import BaseModel, Field

REPO = Path(__file__).parent.parent
dotenv.load_dotenv(REPO / ".env")
# litellm's Gemini (AI Studio) provider reads GEMINI_API_KEY; accept the
# GOOGLE_API_KEY name too since existing Ruach projects use it.
if os.environ.get("GOOGLE_API_KEY") and not os.environ.get("GEMINI_API_KEY"):
    os.environ["GEMINI_API_KEY"] = os.environ["GOOGLE_API_KEY"]

try:
    import litellm
    from litellm import RateLimitError
except ImportError:
    raise ImportError("litellm required: pip install litellm (see pipeline/pyproject.toml)")

logging.basicConfig(level=logging.INFO, format="%(levelname)s: %(message)s")
logger = logging.getLogger(__name__)

MAX_CHUNK_CHARS = 30_000


class SectionAnalysis(BaseModel):
    summary: str = Field(description="2-3 sentence summary of the section, under 100 tokens")
    cross_refs: list[str] = Field(
        default_factory=list,
        description='Other provisions this text cites, e.g. ["Labour Act 2004, s.81", "Workmen\'s Compensation Act"]',
    )


def _response_content(response: Any) -> str:
    if isinstance(response, dict):
        choices = response.get("choices") or [{}]
        return (choices[0].get("message") or {}).get("content") or ""
    choice = (getattr(response, "choices", None) or [None])[0]
    if choice is None:
        return ""
    message = getattr(choice, "message", None)
    if isinstance(message, dict):
        return message.get("content") or ""
    return getattr(message, "content", None) or ""


def _strip_nul(value: str) -> str:
    return value.replace("\x00", "") if value else value


def _build_prompt(chunk_text: str, metadata: dict[str, Any]) -> str:
    context_parts = [f"Act: {metadata.get('source', 'Unknown act')}"]
    if metadata.get("part"):
        context_parts.append(str(metadata["part"]))
    if metadata.get("section_id"):
        context_parts.append(f"Section: {metadata['section_id']}")
    if metadata.get("section_title"):
        context_parts.append(f"Title: {metadata['section_title']}")
    context = " | ".join(context_parts)

    return (
        "You are analyzing one section of a Nigerian statute for a legal "
        "retrieval system.\n\n"
        "Generate a concise 2-3 sentence summary (under 100 tokens) capturing:\n"
        "1. Who or what this provision applies to (e.g. 'workers', 'employers', "
        "'pregnant employees', 'establishments with 25+ staff')\n"
        "2. The key right, duty, prohibition, procedure or penalty it creates\n"
        "3. Any conditions, exceptions, exemptions, provisos or numeric "
        "thresholds (amounts, day counts, time limits) — these matter most\n\n"
        "Also list cross_refs: every OTHER statutory provision this text "
        "cites (other sections of this Act, or other Acts). Use the form "
        "'<Act short name>, s.<n>' where a section is given, or just the Act "
        "name. Empty list if none.\n\n"
        f"Context: {context}\n\n"
        f"Section text:\n{chunk_text}"
    )


async def analyze_chunk(
    chunk_text: str,
    metadata: dict[str, Any],
    model: str,
    max_retries: int = 4,
) -> SectionAnalysis:
    if len(chunk_text) > MAX_CHUNK_CHARS:
        logger.warning(
            f"Truncating chunk to {MAX_CHUNK_CHARS} chars (was {len(chunk_text)}) "
            f"section={metadata.get('section_id') or '?'}"
        )
        chunk_text = chunk_text[:MAX_CHUNK_CHARS]
    prompt = _build_prompt(chunk_text, metadata)

    last_err: Optional[Exception] = None
    for attempt in range(max_retries):
        try:
            response = await litellm.acompletion(
                model=model,
                messages=[{"role": "user", "content": prompt}],
                max_tokens=400,
                response_format=SectionAnalysis,
            )
            return SectionAnalysis.model_validate_json(_response_content(response))
        except RateLimitError as e:
            last_err = e
            wait = min(20 * (2**attempt), 120)
            logger.warning(f"Rate limit, waiting {wait}s (attempt {attempt + 1}/{max_retries})")
            await asyncio.sleep(wait)
        except Exception as e:
            last_err = e
            wait = min(2 * (2**attempt), 30)
            logger.warning(f"LLM call failed: {e}; retrying in {wait}s (attempt {attempt + 1}/{max_retries})")
            await asyncio.sleep(wait)

    raise RuntimeError(f"analyze_chunk failed after {max_retries} attempts: {last_err}")


def _build_augmented_text(metadata: dict[str, Any], summary: str, text: str) -> str:
    """The embedded representation: identity header + summary + statute text."""
    lines: list[str] = []
    for key, label in (
        ("source", "Act"),
        ("part", "Part"),
        ("section_id", "Section"),
        ("section_title", "Title"),
        ("jurisdiction", "Jurisdiction"),
    ):
        val = metadata.get(key)
        if val:
            lines.append(f"{label}: {val}")
    header = "\n".join(lines)

    pieces = [p for p in (header, f"Summary: {summary}" if summary else "", text) if p]
    return "\n\n".join(pieces)


def _chunk_is_done(existing: dict[str, Any]) -> bool:
    return bool(existing.get("summary"))


async def _process_one_chunk(
    chunk: dict[str, Any],
    existing: Optional[dict[str, Any]],
    model: str,
    semaphore: asyncio.Semaphore,
) -> dict[str, Any]:
    if existing is not None and _chunk_is_done(existing):
        return existing

    text = chunk.get("text", "")
    metadata = dict(chunk.get("metadata", {}))

    async with semaphore:
        result = await analyze_chunk(text, metadata, model)

    summary = _strip_nul(result.summary).strip()
    cross_refs = [_strip_nul(r).strip() for r in result.cross_refs if r and r.strip()]
    return {
        "metadata": metadata,
        "text": text,
        "summary": summary,
        "cross_refs": cross_refs,
        "augmented_text": _build_augmented_text(metadata, summary, text),
    }


async def enhance_chunk_file(
    input_path: Path,
    output_path: Path,
    model: str,
    concurrency: int,
    force: bool = False,
) -> None:
    data = json.loads(input_path.read_text(encoding="utf-8"))
    chunks: list[dict[str, Any]] = data.get("chunks", [])
    if not chunks:
        logger.warning(f"No chunks in {input_path.name}, skipping")
        return

    existing_chunks: list[dict[str, Any]] = []
    if output_path.exists():
        try:
            existing_chunks = json.loads(output_path.read_text(encoding="utf-8")).get("chunks", [])
        except json.JSONDecodeError:
            logger.warning(f"Existing {output_path.name} is malformed; reprocessing all chunks")

    aligned_existing: list[Optional[dict[str, Any]]] = [None] * len(chunks)
    if not force and len(existing_chunks) == len(chunks):
        aligned_existing = list(existing_chunks)

    done = sum(1 for ex in aligned_existing if ex is not None and _chunk_is_done(ex))
    if done == len(chunks):
        logger.info(f"⏭️  {input_path.name} fully enhanced ({done} chunks)")
        return
    if done:
        logger.info(f"  Resuming {input_path.name}: {done}/{len(chunks)} already done")

    semaphore = asyncio.Semaphore(concurrency)
    tasks = [
        _process_one_chunk(chunk, aligned_existing[i], model, semaphore)
        for i, chunk in enumerate(chunks)
    ]
    enhanced = await asyncio.gather(*tasks)

    output = {k: v for k, v in data.items() if k != "chunks"}
    output["chunk_count"] = len(enhanced)
    output["chunks"] = enhanced
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(output, ensure_ascii=False, indent=2), encoding="utf-8")
    logger.info(f"✓ Enhanced {input_path.name} ({len(enhanced)} chunks)")


def main() -> None:
    parser = argparse.ArgumentParser(description="Enrich statute chunks with SAC summaries + cross-refs")
    parser.add_argument("--input-dir", type=Path, default=REPO / "data/chunks")
    parser.add_argument("--output-dir", type=Path, default=REPO / "data/chunks-sac")
    parser.add_argument("--model", type=str, default="gemini/gemini-2.5-flash-lite")
    parser.add_argument("--limit", type=int, help="Process only first N files (for testing)")
    parser.add_argument("--force", action="store_true", help="Regenerate even completed chunks")
    parser.add_argument("--concurrency", type=int, default=20, help="Max in-flight LLM calls")
    args = parser.parse_args()

    files = sorted(args.input_dir.glob("*.json"))
    if not files:
        raise SystemExit(f"No JSON files in {args.input_dir}")
    if args.limit:
        files = files[: args.limit]
    logger.info(f"Processing {len(files)} files (concurrency={args.concurrency}, model={args.model})")

    async def process_all() -> None:
        for path in files:
            logger.info(f"📄 {path.name}")
            await enhance_chunk_file(path, args.output_dir / path.name, args.model,
                                     args.concurrency, args.force)

    asyncio.run(process_all())
    logger.info(f"✅ Done! Enhanced chunks in {args.output_dir}")


if __name__ == "__main__":
    main()
