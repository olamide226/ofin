"""Regression tests for the statute chunker.

These run against the real committed corpus (fast and deterministic) and pin
the exact failure modes found during Week 2:
  - last section swallowing schedules (ECA s.74 bug)
  - TOC runs mistaken for the body
  - repeal gaps rejected as sequence breaks (TDA ss.20-32)
  - transcription dropouts (Labour Act s.80, T9 ss.1/2/7)
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))

import pytest
from chunk_statutes import DOMAINS, chunk_act

RESULTS = {cfg.act_short: chunk_act(cfg, "test") for cfg in DOMAINS["labour"]}


def sections(act: str) -> set[int]:
    return {
        c["metadata"]["section_number"]
        for c in RESULTS[act]["chunks"]
        if c["metadata"]["section_number"] is not None
    }


def test_labour_act_complete():
    assert sections("Labour Act 2004") == set(range(1, 93))


def test_labour_act_s11_identity():
    chunks = [c for c in RESULTS["Labour Act 2004"]["chunks"]
              if c["metadata"]["section_id"] == "s.11"]
    assert len(chunks) == 1
    meta = chunks[0]["metadata"]
    assert meta["section_title"] == "Termination of contracts by notice"
    assert "one month, where the contract has continued for five years" in chunks[0]["text"]


def test_eca_schedules_not_swallowed_by_last_section():
    s74 = [c for c in RESULTS["Employees' Compensation Act 2010"]["chunks"]
           if c["metadata"]["section_id"] == "s.74"]
    assert len(s74) == 1, "ECA s.74 must be a single chunk, not the schedule dump"
    scheds = [c for c in RESULTS["Employees' Compensation Act 2010"]["chunks"]
              if c["metadata"]["section_id"].startswith("sch.")]
    assert len(scheds) >= 2, "ECA compensation schedules must chunk separately"


def test_tda_repeal_gap_preserved():
    nums = sections("Trade Disputes Act 2004")
    assert 19 in nums and 33 in nums and 52 in nums
    assert not nums & set(range(20, 33)), "ss.20-32 were repealed by NIC Act 2006"
    assert {5, 6, 7, 8} <= nums, "deep-indented body headings must be found"


def test_tda_chunks_are_body_not_toc():
    """Regression: the TOC run once outscored the fragmented body run and
    every 'section' chunk was a one-line TOC entry."""
    chunks = {c["metadata"]["section_id"]: c["text"]
              for c in RESULTS["Trade Disputes Act 2004"]["chunks"]
              if c["metadata"]["chunk_type"].startswith("section")}
    assert "strike" in chunks["s.18"] and len(chunks["s.18"]) > 400
    assert "(1)" in chunks["s.4"], "body sections carry subsection prose"


@pytest.mark.parametrize("act", list(RESULTS))
def test_mean_section_length_is_body_scale(act):
    """A TOC-chunked act averages ~60 chars/section; real bodies are far
    longer. Guards every act against wholesale mis-detection."""
    texts = [c["text"] for c in RESULTS[act]["chunks"]
             if c["metadata"]["chunk_type"].startswith("section")]
    mean = sum(map(len, texts)) / len(texts)
    assert mean > 300, f"{act}: mean section text {mean:.0f} chars — TOC-chunked?"


def test_t9_reconstructed_sections_present():
    assert sections("Trade Disputes (Essential Services) Act 2004") == set(range(1, 9))


def test_nmw_complete_with_amendment_text():
    assert sections("NMW Act 2019") == set(range(1, 19))
    s3 = next(c for c in RESULTS["NMW Act 2019"]["chunks"]
              if c["metadata"]["section_id"] == "s.3")
    assert "N70,000.00" in s3["text"], "2024 amendment figure must be in s.3"


@pytest.mark.parametrize("act", list(RESULTS))
def test_every_chunk_has_citation_identity(act):
    for c in RESULTS[act]["chunks"]:
        m = c["metadata"]
        assert m["act_short"] and m["section_id"], "verifier lookup key must exist"
        assert c["text"].strip()
