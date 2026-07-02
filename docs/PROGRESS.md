# Progress Log

## Week 6 (Aug 5–11, started early July 2) — Hardening + polish

| Task | Status | Notes |
|---|---|---|
| Web UI revamp (world-class chat experience) | done | Complete `index.html` rewrite: chat thread with multi-turn history, live pipeline timeline (search → understand → write → verify) collapsing to a summary line, source chips that expand to the statutory text, serif answer typography with inline citation chips that jump to their receipt, verification section with ✓/!/✕ pills and expandable receipt cards, deterministic-computation card with band table, light/dark themes, sticky composer with stop button. Still vanilla HTML/CSS/JS in one `go:embed` file (ADR-011 unchanged). Server fixes: `retrieved` SSE event now carries source text (chips can show the law); `computed` event now sends the real `ComputationHTML` table (was sending plain text twice). Browser-tested: PAYE computation, lookup with receipts, citation→receipt jump, dark mode — zero console errors. |
| UI-exposed corpus polish item | noted | Source panels expose gazette margin-note artifacts in NTA chunk text (e.g. s.58 shows "Act No. 8, 2019" fragments) — display-side cleanup or chunker-side margin stripping to evaluate |

Remaining Week 6 worklist: cross-act hop edges, "18 months" extractor,
top-N tuning, Pidgin answer toggle, golden set → 90, test_prompts selection,
evidence pack capture.

## Week 5 (July 29–Aug 4, started early July 2) — Performance + UX

| Task | Status | Notes |
|---|---|---|
| App extraction | done | `engine/internal/app` — `App.Ask` with typed Emitter callbacks; CLI is a thin adapter; web UI is a symmetrical consumer |
| Web UI (`ofin serve`) | done | `engine/internal/web` — single `go:embed` HTML file, vanilla HTML/CSS/JS, no build toolchain (ADR-011); SSE `/api/ask`; retrieval preview, streaming tokens, expandable receipt cards, computation tables |
| Prompt diet | done | SystemPrompt -30%; hop-companion sources trimmed to 800 chars (fused stay at 3000); context halved to 4096 |
| KV-cache quantization | done | `-ctk q8_0 -ctv q8_0` for the chat server |
| Speculative decoding | rejected (ADR-012) | Llama 3.2 1B draft added ~1.6 GB peak RSS (5.3 GB total); off by default; `--draft` opt-in for demo machines |
| Diet evaluation | **passed** | recall 76% (=baseline), precision 78% (−1pt, within guard). Claims shown 148 (−14; less noise), failed 4 (−2); refusal improved 65 (+4). Diet is safe. |

### Benchmark snapshot (2026-07-02, M1 Max 64 GB — dev-only per ADR-002)

| Config | Chat RSS | Embed RSS | Total stack | Context | KV cache |
|---|---|---|---|---|---|
| Post-diet, no draft | ~3.7 GB | ~181 MB | ~3.9 GB | 6144 | q8_0 |
| With 1B draft | ~5.3 GB | ~181 MB | ~5.5 GB | 6144 | q8_0 (×2) |

Draft rejected for target ship (ADR-012). The diet numbers (recall/precision)
are the new baseline for Week 6 hardening.

### VM certification (2026-07-02, AMD EPYC 4 vCPU / 7.6 GB, Ubuntu 26.04)

| Metric | Value | vs Dev (M1 Max) |
|---|---|---|
| TPS generation | **34.29** | 50.1 (both cap at 15 for S_perf) |
| First-token latency | 5,977 ms | 910 ms |
| Peak RSS | **3,442 MB** | 2,110 MB |
| Steady-state RSS | 3,323 MB | 1,975 MB |
| Throttled | false | false |

**S_perf: full marks** (34.29 > 15 TPS cap). **S_eff: passing** (3,442 MB < 3.5 GB target). Real-world first-token latency with 3–4k token prompts expected 10–15s on the VM — the UI masks this with instant retrieval preview.

### Remaining Week 5 items

- [x] VM certification run — **done** (2026-07-02, `docs/benchmarks/2026-07-02-vm-llama3.2-3b-4vcpu8gb.json`)
- [ ] `ofin serve` screenshot capture for docs
- [ ] Prompt budget final: the 148 claims figure suggests further trimming the number of retrieved sources could help (fewer, more targeted chunks → fewer uncited claims → less prefill) — evaluate in Week 6 

## Week 4 (July 22–28, started early July 2) — Rules engine + router

## Week 4 (July 22–28, started early July 2) — Rules engine + router

| Task | Status | Notes |
|---|---|---|
| Cross-reference retrieval hop | done | recall@k 81% → **84%**; bidirectional (statutes cite backwards); hops extend context (6 fused + ≤2 companions) |
| Verify current tax rates | done | Nigeria Tax Act 2025 in force since 2026-01-01: 0% ≤₦800k → 15/18/21/23/25% at 3M/12M/25M/50M; rent relief 20% capped ₦500k; CRA gone; minimum-wage earners exempt. Sources: PwC WWTS (rev. 2026-05-29), KPMG FA 2025-168. Encode-vs-gazette check pending tax corpus |
| Rules engine: notice + PAYE | done | `engine/internal/rules`, table-driven tests with hand-computed values; version-stamped |
| Intent router | done | extraction (bake-off X-format) → rules engine → **deterministic rendering** (ADR-010: model never touches figures) |
| Week-4 exit criterion | **met** | "how much PAYE on 450k monthly" returns the exact statutory computation with band breakdown (₦63,500/mo, 3 bands) |
| Integrated golden run (2026-07-02T113658 + gated spot-run) | recall@k **84%** · citation precision **91%** (52/57) · refusal calibration **39/40** · notice/PAYE routed to rules engine, rendered deterministically (ADR-010) | intent gates added after two presence-of-numbers misroutes (L15, N02) — now regression-tested |
| Tenancy + tax corpora | **done** | Lagos Tenancy Law 2011 (official MOJ gazette, 48/49 sections) + Nigeria Tax Act 2025 (official gazette via GRS mirror — the NTA consolidated PITA/CITA/VAT/CGT, so one act covers the domain; 202/203 sections). Corpus now **523 chunks, 7 acts, 3 domains** |
| PAYE bands vs gazette | **✅ verified** | Fourth Schedule text (under s.58(1)) matches the rules engine exactly |
| Golden set | 68 questions (39 labour + 29 tenancy/tax incl. 5 cross-domain, 6 computation, 5 negatives) | → 90 during Week-6 hardening, driven by observed failures |

### Three-domain baseline (2026-07-02T124409) — the Week-6 worklist

recall@k **76%** · citation precision **79%** (128/162) · refusal 61/68 ·
7 questions computed deterministically. Expected regression from the corpus
tripling; failures concentrate in: (1) **cross-domain recall 1/5** — the
cross-ref hop is same-act-only; cross-act edges need act-name resolution in
`reverseRefIDs`; (2) tax-prose claims flag more often (28); (3) extraction
misses "18 months" tenure (CP05 gracefully fell back to a correct cited
lookup). XD03 proves the pipeline end-to-end: both acts retrieved, rent
deductibility answered with [NTA 2025, s.20(1)(b)].

## Week 3 (July 15–21, started early July 2) — Verified Citation Engine

| Task | Status | Notes |
|---|---|---|
| Citation grammar + parser | done | `engine/internal/verify/parse.go` — `[Act, s.X(Y)]` tokens, claim segmentation |
| Verifier | done | 3 layers (ADR-009): existence → quantity consistency (question-echo aware) → semantic support (calibrated thresholds 0.66/0.55) |
| Regeneration loop | done | single constrained retry with failure reasons + correct statutory text injected; live-caught the "7 days vs 14 days" hallucination on first e2e run |
| Eval harness | done | `eval/run_golden.py` — one command; JSON output from `ofin -json`; results archived in `eval/golden/results/` |
| Retrieval recall@6 baseline | **81%** (30/37) | misses: 3 multi-section questions (cross-ref hop lands Week 4), 4 rank-7+ near-misses (Week 6 tuning) |
| Citation precision | **90%** (60/67 verified; 5 ⚠, 2 ✗, all explicitly marked) | exit criterion met in spirit: every UI-visible claim verified or flagged. Prose-citation parsing tripled coverage (27→67-81 claims/run) — precision is honest, not format-cherry-picked |
| Refusal calibration | 38/40 | both misses are retrieval gaps (L11 s.15 rank>6; E07 needs s.13+s.14), not refusal logic |
| Profiler regression under sustained load | pending | end of Week 3 |

**Verifier limitation (documented, ADR-009):** wrong-band claims from
graduated tables ("4 years → one month") are invisible to all three layers —
"one month" genuinely appears in s.11. Architectural mitigation: the Week-4
intent router sends tenure/banded computations to the rules engine.

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
