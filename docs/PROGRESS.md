# Progress Log

## Week 1 (July 1–7, 2026) — Foundations and bake-off

Target exit criteria: model locked, hardware in hand, labour corpus in clean
text, baseline numbers recorded.

| Task | Status | Notes |
|---|---|---|
| Read DevPost rules, eligibility, IP terms | done | ⚠️ residency requirement — see action items below |
| Register on DevPost | **blocked on Ola** | needs personal account; yields `team_id` |
| Clone submission template + profiler | done | siblings of this repo in `ai_world/` |
| Secure 8 GB target hardware | **blocked on Ola** | refurb ThinkPad class, ~£120–180 |
| Model bake-off (Qwen 2.5 3B / Llama 3.2 3B / Phi-3.5-mini) | **done — Llama 3.2 3B locked** | ADR-006; scoresheet in `eval/bakeoff/SCORES.md` |
| Baseline profiler run (locked model) | done | 55.4 TPS / 902 ms / 2107 MB peak RSS on M1 Max (dev baseline, ADR-002) |
| Source + clean labour corpus | done | 5 acts in `corpus/labour/`, provenance in `sources.md` |
| Name check "Òfin" | done | no collision found (ADR-005) |

### Ola action items — status July 1 (evening)

1. ~~Eligibility~~ **resolved**: entering via Ruach Tech (Lagos-HQ,
   incorporated 2025) — ADR-007.
2. DevPost registration **done**. Still needed: the ADTF portal `team_id`
   and GitHub handle for `metadata.json` (the /joins/ URL is a team invite
   link — keep it private, it is not the team_id).
3. ~~Refurb laptop~~ **dropped by decision**: certification on an 8 GB
   RAM-capped VM (ADR-007).

## Week 2 (July 8–14, started early July 1) — Labour vertical slice

| Task | Status | Notes |
|---|---|---|
| Port SAC pipeline from Immigranta | done | `pipeline/enhance_chunks_with_sac.py`; prompts adapted for statutes; cross_refs extracted at enrichment time |
| Statute-aware chunker | done | 230+ sections across 6 docs; run-based body detection; repeal-gap handling |
| SAC enrichment run over labour corpus | done | 259 chunks, 161 cross-ref edges (Gemini 2.5 Flash-Lite, build-time) |
| Ingest into SQLite-vec with metadata | done | `pipeline/ingest.py` → `data/ofin.db` (245 chunks, 2.8 MB, single file); bge-small embeddings (ADR-008) |
| Hybrid retrieval (vector + FTS5, RRF) | done | Go `engine/`; 10–25 ms retrieval; RRF k=60, top-6 |
| End-to-end CLI (question → cited answer) | done | `ofin ask` — llama-server SSE streaming, ~5 s answers (dev machine), fully offline. **Week 2 exit criterion met** |
| Golden eval set (40 labour Q&A) | done | `eval/golden/labour.jsonl` — sections verified against chunked corpus; 3 negative controls; harness lands Week 3 |

### Week 2 findings that shape Week 3

1. **Verifier failure corpus captured** (all reproducible via `ofin ask`):
   tenure→notice-band mis-mapping ("4 years → one month"), self-contradiction
   within one answer, invented citation (`s.7(8)`), invented specifics
   ("7 days written" vs s.4's actual 14 days). These become verifier unit
   tests.
2. **Refusal calibration is two-sided**: strict prompt rules made the model
   refuse answerable "what can I do?" questions. Fixed with partial-answer
   duty in the system prompt; golden set now tests both directions.
3. **Chunker: schedules need their own identity** — ECA compensation scales
   were mislabeled s.74 before the fix, poisoning retrieval.
4. Known title-quality issues: some T8/ECA marginal titles garbled
   (`s.43 "danger to persons or property"`); title QA pass scheduled Week 3
   accuracy hardening.
