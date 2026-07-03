# Progress Log

## Week 6 (Aug 5–11, started early July 2) — Hardening + polish

| Task | Status | Notes |
|---|---|---|
| Web UI revamp (world-class chat experience) | done | Complete `index.html` rewrite: chat thread with multi-turn history, live pipeline timeline (search → understand → write → verify) collapsing to a summary line, source chips that expand to the statutory text, serif answer typography with inline citation chips that jump to their receipt, verification section with ✓/!/✕ pills and expandable receipt cards, deterministic-computation card with band table, light/dark themes, sticky composer with stop button. Still vanilla HTML/CSS/JS in one `go:embed` file (ADR-011 unchanged). Server fixes: `retrieved` SSE event now carries source text (chips can show the law); `computed` event now sends the real `ComputationHTML` table (was sending plain text twice). Browser-tested: PAYE computation, lookup with receipts, citation→receipt jump, dark mode — zero console errors. |
| UI-exposed corpus polish item | noted | Source panels expose gazette margin-note artifacts in NTA chunk text (e.g. s.58 shows "Act No. 8, 2019" fragments) — display-side cleanup or chunker-side margin stripping to evaluate |
| **Ship blocker fixed** (found by Ola's VM full-stack run) | done | Fresh clones crashed: `DefaultConfig` set a draft-GGUF path that `download_model.sh` never ships → chat server died ("not healthy after 90s"), contradicting ADR-012. Fixed: draft off by default, opt-in via `ofin -draft`; missing draft GGUF now non-fatal (warns, continues without); post-subcommand flags now parsed (`ofin ask -json "q"` works — Go's flag pkg stops at the first positional); real `-port` flag for serve. `bench_ask.sh` updated. VM evidence: `docs/benchmarks/2026-07-02-vm-integrated-fullstack.json` |
| Full-stack VM measurement (Ola) | recorded | Whole stack on 4 vCPU/7.6 GB: ~3.9 GB idle (q8 KV pre-allocated), 4.2 GB peak warm under load — over the 3.5 GB self-target by ~0.4–0.7 GB but well inside the 8 GB hard cap, no swap/OOM. Sustained TPS 34.0/34.5/34.0 (no throttle decay). Offline check PASS (zero non-loopback connections during generation). Warm computation 1.4s; warm RAG+verification 143s — verification re-embedding latency is a Week 6 optimisation candidate |
| Chat history + sidebar | done | Conversations persist in `localStorage` (privacy-preserving: never leaves the device, matches the offline pitch). Sidebar with New chat / switch / delete, relative timestamps, quota-aware eviction (oldest conversations dropped first, then oldest turns; 30-turn cap per conversation). Mobile: slide-over sidebar with backdrop. Restored turns re-render sources, computation cards, answers, and receipts from the stored record — no re-generation. Conversational *memory* (model seeing prior turns) deliberately excluded pre-freeze: it would eat the 6144 prompt budget, need query rewriting, and break verifier claim-independence — parked post-challenge. |

| Cross-act reverse-reference edges | done | `retrieve.buildRefIndex` — every chunk's SAC cross_refs resolved once into an in-memory index (prose act names → canonical act_short via the same `refTargetAct` semantics both directions). Reverse lookup now crosses acts: same-act sectioned edges first, then cross-act sectioned, then act-level (NTA s.58 → "Minimum Wage Act", the edge cross-domain questions travel). Fixed latent bug both directions: refs naming acts OUTSIDE the corpus ("Pension Reform Act 2014, s.4") used to default to the seed act, minting bogus edges — now skipped. Spot-check: XD05 now retrieves NTA s.58 rank 3; XD03 both expected sections ranks 1–2 |
| "18 months" extractor (CP05) | done | Two layers: extraction-prompt hint (months → fractional years) + deterministic fallback — when the model extracts no tenure, the question's first explicit duration ("18 months", "three years", digits or words) is parsed in Go. Precedence: start date > model years > question regex. Table-driven tests incl. Pidgin phrasing |
| Pidgin answer toggle | done | `ofin -pidgin` flag + web UI pill toggle (persisted): appends `answer.PidginDirective` to the system prompt — answers in Pidgin whatever language the question used, citations stay bracket-format for the verifier. Targets +15% African Alpha / Best Localisation |

### 90-question eval — three calibration rounds (2026-07-03)

| Metric | v1 | v2 | v3 |
|---|---|---|---|
| Recall (90-Q set) | 63% | 70% | **70%** |
| Recall (old-68 subset) | 71% | 78% | **78%** |
| Citation precision | 77% | 82% | **84%** |
| Failed-after-regen | 20 | 13 | **7** |
| Refusal calibration | 85/90 | 85/90 | **88/90** |
| Rules-engine routed | 15 (2 misroutes) | 11 | **13 (0 misroutes)** |

Per-round fixes: v1→v2 routing guards (tenancy veto, question-evidence for
figures), retrieval tuning (8 fused / 4 full-text, breadth-first hops),
golden-set corrections (XD01/TX07 cited derivation rules instead of the s.50
severance exemption / NTAA s.14 PAYE machinery), regeneration context-overflow
fix. v2→v3 "and a half" durations, spelled-figure evidence, narrower refusal
trigger + partial-answer duty, shape-based refusal detection in the harness.

**Known remaining misses (documented, ranked):**
1. Cross-domain recall 2/11 — structural; needs query decomposition (post-freeze candidate) or better cross-act seeding
2. TX03 "what are the tax bands" refuses — s.58 never seeds (NTA margin-note artifacts pollute its embedding; the corpus-polish item is now load-bearing). Schedule-ordinal hop resolution landed (sch.7 follows s.58 whenever s.58 seeds)
3. E02 — ECA s.5 missing from corpus (transcription dropout; needs source gazette)
4. L19 (s.80) / N02 (s.4) — hop-quota lottery; principled fix is a reranker
5. N03 — Pidgin phrasing retrieves poorly against statutory English (Pidgin query normalization, Week 7 candidate)

### NTA margin-note cleanup — scoped, deferred to Week 7 (2026-07-03)

Investigated the artifact that makes TX03 ("current tax bands") refuse: NTA
s.58's text is polluted by `pdftotext -layout` interleaving right-margin
shoulder-notes ("Rates of tax / Act No. 8, 2019 / Fourth Schedule") into the
body, wrecking its embedding. A conservative right-margin de-margin cleans it
(s.58 701→255 chars, clean) — but the gazette layout is **bimodal**: other
sections carry LEFT-margin notes (s.163 "Thirteenth Schedule", s.57 "Effective
Tax Rate") a right-margin rule can't touch. A complete, safe fix is a converter
pass over ~388 tax chunks with over-stripping risk — a Week-7 corpus-polish job,
not worth grinding for one question mid-hardening. Working right-margin
function saved for the restart:
`re.compile(r'^(.{55,}?)\s{6,}(\S.{0,45})$')` → keep group(1). Left-margin needs
a column-detection companion.

### Plan-audit closures (2026-07-04)

| Item | Status | Notes |
|---|---|---|
| Pillar 2 completion: tenancy + redundancy calculators | **done** | Rules engine now has 4 computation kinds. Tenancy: s.13(1) bands by tenancy type (deterministic type parse from question, per the input-evidence rule), arrears-lapse notes s.13(2)/(3), fixed-term s.13(5) 7-day rule, stipulation caveat + Lagos jurisdiction flag. Redundancy: **corrected the plan's assumption after reading the gazette — s.20 prescribes NO severance formula** (duty to negotiate, s.20(1)(c)); encoded as an honest entitlement breakdown (union notice, LIFO, negotiated pay) + the s.11 notice band from tenure. Kind normalization re-routes tenancy-notice questions the extractor mislabels. Guard eval: TN01+TN02 now compute deterministically, 14/19 routed, no regressions |
| Tri-language spike (Yoruba/Hausa/Igbo) | **rejected — ADR-017** | Retrieval collapses on non-English queries AND Llama 3.2 3B cannot translate them (hallucinated translations at temp 0). Mistranslation → confident wrong legal answer = harm. African-language claim scoped to Pidgin only, honestly |

### Week 6 close-out

Landed: web UI + chat history, verifier latency cache, cross-act + schedule
hop edges, extractor fallbacks, Pidgin toggle, NTAA (8th act), SAC alignment
fix, 90-Q golden set + 3 calibration rounds (old-68 recall 78% / precision 84%),
routing evidence guards, VM certification (f16+fa ADR-014), configurable packing
(ADR-015), Gemma-4 spike rejected on memory (ADR-016).

Deferred to Week 7 (submission packaging): NTA margin cleanup (above),
test_prompts selection, evidence pack, Pidgin eval set (unlock for the
fine-tuning differentiation thread). Dropped: ctx 6144→4096 (would worsen the
regeneration overflow — see ADR-014 prefill note).

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
| TPS generation (llama-bench, sustained 3×) | **34.29** (34.0 / 34.5 / 34.0) | 50.1 (both cap at 15 for S_perf) |
| First-token latency | 5,977 ms | 910 ms |
| Peak RSS — model only (llama-bench) | 3,442 MB | 2,110 MB |
| **Peak RSS — full app stack (integrated, draft off)** | **~3.9 GB idle / ~4.2 GB peak** | — |
| Throttled | false | false |

**S_perf: full marks** (34.29 > 15 TPS cap, no throttle decay across 3 runs).
**S_eff: corrected — see ADR-013.** The model-only 3,442 MB previously logged as
"passing" was measured with llama-bench, which never launches the app. The real
shipped stack (embed + chat @ 6144-ctx q8 KV + Go engine + SQLite) is ~3.9 GB
idle / ~4.2 GB peak — **over** the 3.5 GB self-target, but within the 8 GB hard
cap (no swap/OOM). **Offline** (zero non-loopback calls) and **functional**
(cited answers, both routes) verified end-to-end.

✅ **Ship-blocker found on VM, fixed and verified (commit `ff9e776`):** `ofin ask`
/ `serve` were dead out-of-the-box — `DefaultConfig` shipped the 1B draft on but
only the 3B is downloaded, so the chat server crashed; and the escape-hatch flag
was ignored after the subcommand. Both violated ADR-012. Fix landed: draft off by
default (`app.go:42`), opt-in `--draft` with post-subcommand flags re-parsed
(`main.go`), and a missing draft GGUF made non-fatal (`client.go`). Re-verified on
the dev machine — default `ofin ask` returns cited answers on both routes, chat
server launches with no `--model-draft`. See ADR-013.

### Remaining Week 5 items

- [x] VM certification run — **done** (2026-07-02; integrated:
  `docs/benchmarks/2026-07-02-vm-integrated-fullstack.json`, llama-bench:
  `2026-07-02-vm-llama3.2-3b-4vcpu8gb.json`)
- [x] **ADR-012 draft-off fix landed** (commit `ff9e776`) — draft off by default,
  opt-in `--draft`, post-subcommand flags parsed, missing draft non-fatal;
  re-verified on dev machine — see ADR-013
- [x] Corrected S_eff figures in REPORT.md to the integrated stack numbers
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
