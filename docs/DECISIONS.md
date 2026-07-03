# Decision Log

Architecture Decision Records for Òfin. Newest first. Every entry states the
decision, the alternatives considered, and why. This log feeds REPORT.md
("Design Decisions") and Gate 2 Q&A prep.

---

## ADR-014 — Chat server runs f16 KV + flash attention, not q8_0 KV; the real latency lever is prompt-prefill size (2026-07-03)

**Decision:** The app's chat `llama-server` runs with **`-fa on`** (flash
attention, f16 KV cache), replacing the shipped `-ctk q8_0 -ctv q8_0`.

**Context:** Second VM run (Hetzner 4 vCPU / 7.6 GB, llama.cpp b9864,
2026-07-03) — the first to actually measure *app* latency on target hardware.
Warm lookup answers took 90–190s, barely better than the pre-cache 143s. A/B/C/D
benchmark over four fixed questions, first-generation wall time:

| Config | Q1 | Q2 | Q3 | chat RSS |
|---|---|---|---|---|
| **B — f16 + fa (chosen)** | **65.5s** | **108.9s** | **75.0s** | 3974 MB |
| C — f16, no fa | 78.8s | 131.1s | 77.7s | 3973 MB |
| D — q8_0 + fa | 94.0s | 180.8s | 109.7s | 3659 MB |
| A — q8_0, no fa (was shipped) | 112.1s | 192.9s | 119.4s | 3659 MB |

**Findings:**
- **f16 KV is ~40% faster than q8_0 on CPU** (B/C beat D/A). Dequantizing the
  cache on every attention step has no hardware acceleration on the CPU-only
  audit box — the opposite of the M1 dev machine, where Metal made it free. This
  is why q8_0 (added for memory, ADR-002-era) looked fine in dev and is
  pathological on target. Flash attention adds a smaller gain on top.
- **Cost of the switch is +315 MB** (3974 vs 3659), well inside the 8 GB cap, and
  **zero score impact** — the audit profiles the raw GGUF, not our server
  (ADR-003). f16 KV is also *higher* fidelity than q8_0, so no accuracy risk.
- **The KV format is not the root cause.** Answers are ~100–160 tokens (~5s to
  decode at 34 TPS), yet first-gen is 65–190s. The gap is **prefill** of the
  ~4,000-token prompt (8 statutory sources). Latency is prompt-size-bound; the
  recall tuning that grew us to 8 sources / 4 full-text (2026-07-03) is the
  direct cause. This is the accuracy↔latency tension, and on CPU it dominates.

**Consequences / next:** f16+fa is a free ~40% win, taken. The larger lever —
**prompt prefill size** — is the next latency investigation: test 2-full +
N-summary source packing (SAC summaries are far shorter than full text) against
the recall baseline, on the VM. Streaming does NOT mask this: the retrieval
preview shows at 28ms but the user then waits through the full prefill before the
first answer token. Documented honestly; this is the open demo-UX risk.

**Alternatives considered:** keep q8_0 for memory (rejected — memory isn't the
constraint, latency is, and the memory doesn't score); q8_0+fa to keep the saving
(rejected — config D still 40% slower than B). A base-model swap to a
sliding-window-attention model (Gemma 4 E4B) would attack prefill architecturally
but reopens the ADR-006 Pidgin decision — parked as a separate bake-off spike.

## ADR-013 — VM integrated certification: full-stack RSS is the S_eff figure of record; ADR-012 was not implemented in code (2026-07-02)

**Decision:** The S_eff certification figure is the **integrated full-stack peak
RSS** from running the actual `ofin` app on the target VM — not the model-only
`llama-bench` number. Figure of record: **~3.9 GB steady / ~4.2 GB peak** (draft
off), superseding the 3,442 MB model-only value previously logged as "S_eff
passing."

**Context:** First real VM run — Hetzner 4 vCPU / 7.6 GB, Ubuntu 26.04,
2026-07-02. Prior certification profiled only the raw GGUF via llama-bench and
never launched the application.

**Findings:**
- **The app was non-functional out-of-the-box.** `ofin ask` / `ofin serve` chat
  server died ("llama-server on port 8092 not healthy after 90s"). Two bugs that
  together violate ADR-012:
  1. `DefaultConfig` ships the 1B draft model **on** (`app.go:39`), but
     `download_model.sh` fetches only the 3B — so `--model-draft <missing-file>`
     crashes the chat server. ADR-012 says draft is **off** by default; the code
     did the opposite.
  2. The escape hatch is a broken opt-out: `ofin ask --no-draft` is silently
     ignored because Go's `flag` package stops at the first positional
     (`main.go:41-52`) — only `ofin --no-draft ask` works. ADR-012 specifies
     draft behind a flag (opt-**in**), not opt-out.
- **Fix — landed in commit `ff9e776` and re-verified on the dev machine.**
  Three parts: (1) `DefaultConfig.DraftModel = ""` (draft off by default,
  `app.go:42`); (2) flag flipped to opt-in `--draft` plus a re-parse of the
  post-subcommand args so flags after `ask`/`serve` are honored
  (`main.go:42,54-61`); (3) a missing draft GGUF is now non-fatal — `client.go`
  `os.Stat`-guards `--model-draft` and continues without it
  (`client.go:57-65`). Verified: default `ofin ask "…"` returns cited answers
  on both the rules-engine and RAG-generation paths (exit 0), and the chat
  server launches with **no** `--model-draft` in its args.
- **S_eff reality:** model-only 3.44 GB is under the 3.5 GB self-target, but the
  shipped stack (embed + chat @ 6144-ctx q8_0 KV + Go engine + SQLite) is
  ~3.9 GB idle / ~4.2 GB peak — over the 3.5 GB self-target, within the 8 GB hard
  cap (no swap, no OOM). RSS ≈ PSS (little sharing) → the figure is honest.
- **S_perf:** 34 TPS, sustained across 3 back-to-back runs, no throttle decay.
- **Offline:** zero non-loopback connections during real generation.
- **Functional:** correct cited answers on both RAG and deterministic-computation
  routes; verifier receipts working.

**Why this matters:** llama-bench certification is necessary but not sufficient —
it cannot catch a broken application. Certification must run the app end-to-end
on target hardware.

**Data:** `docs/benchmarks/2026-07-02-vm-integrated-fullstack.json` (integrated)
and `2026-07-02-vm-llama3.2-3b-4vcpu8gb.json` (llama-bench).

**Note on ADR-012's revisit trigger:** its trigger ("only if VM cert shows the
3B alone overshoots 3.5 GB") is **not** hit — the 3B alone is 3.44 GB. But the
full stack overshoots the 3.5 GB self-target; if S_eff scoring pressure grows,
revisit KV ctx (6144→lower) or embedding-model size before re-adding the draft.

## ADR-012 — Speculative decoding off by default; RSS cost too high (2026-07-02)

**Decision:** Not on by default. Available behind `ofin` CLI flag only.

**Evidence:** Measured 2026-07-02 on dev machine (M1 Max 64G): Llama 3.2 1B
Q4_K_M draft (770 MB) + main model (1.9 GB) + dual KV caches in q8_0 added
~1.6 GB RSS vs baseline — peak ~5.3 GB vs ~3.7 GB without draft. The plan
budgeted 0.4 GB for the draft; the underestimation came from KV cache
doubling (two models' context states) and macOS allocator generosity at 64 GB
RAM. On a real 8 GB machine the draft would likely push RSS past the
disqualification ceiling.

**Measured TPS impact on M1 Max:** none measured (computation path is
instant; lookup path is already 50+ TPS, above the 15-TPS S_perf cap per
ADR-003). The draft is a net-negative: costs real RSS for a TPS gain the
scoring formula cannot reward.

**Revisit trigger:** only if VM certification shows the 3B alone overshoots
3.5 GB and we downsize to a 1B-class model, at which point the 0.5B draft
(not this 1B) might be re-evaluated.

## ADR-011 — Web UI: vanilla static assets, no build toolchain (2026-07-02)

**Decision:** The local web UI is a single `go:embed`ded HTML file with
vanilla CSS/JS — no TypeScript, no bundler, no framework, no npm. SSE
streaming over one `/api/ask` endpoint.

**Why:**
1. Reproduction simplicity is a judged criterion ("a stranger can clone and
   run in under 15 minutes"). Nothing defeats that like `npm install`.
2. Only two dependencies: the browser's `fetch` and `EventSource`-style
   streaming. Both ship in every modern browser.
3. The `App.Ask` emitter interface already separates the pipeline from the
   presentation — CLI and web are symmetrical 1:1 consumers of the same
   typed event stream.

**What this means for the future:** Tauri or any bundled shell is rejected
for the submission. If visual polish is needed, it happens inside the single
HTML file (or at most, a few additional embedded assets). Week-6 localisation
(Pidgin labels, translated UI strings) stays plain-text in the same file.

## ADR-010 — Computation answers render deterministically; the LLM never touches figures (2026-07-02)

**Context:** The plan's Pillar 2 said "the LLM narrates the result". Tested
live, Llama 3.2 3B **recomputed numbers it was explicitly told to
transcribe**: handed a computed PAYE of ₦63,500/month in JSON with "use
EXACTLY these numbers", it invented a 7.78% rate and answered ₦35,000 —
twice, including once with a fabricated citation. The verifier caught the
invention (quantity layer), but catch-and-retry cannot converge on a model
that keeps doing arithmetic.

**Decision:** The rules engine's numeric core renders **deterministically
in Go** (`rules.NoticeResult.Render` / `PAYEResult.Render`): outcome, band
breakdown, citations, statutory version stamp. The model's only future
role on this path is optional phrasing AROUND the rendered block (e.g.
Pidgin restatement, Week 6) — never producing text containing the figures.
Receipts for computed answers are verified by construction.

**Bonus effects:** computation answers are instant (no generation pass),
identical every run, and immune to prompt-injection via retrieved chunks.

**Also decided:** deterministic date arithmetic outranks model-extracted
durations — the extractor invented `employment_years: 3` for "since March
2020" (truth: 6.3 years). When both a start date and a duration are
extracted, the start date wins and Go computes the tenure.

## ADR-009 — Verifier architecture: deterministic layers first (2026-07-02)

**Decision:** The Verified Citation Engine checks each cited claim in three
layers, ordered by trustworthiness:

1. **Existence** (deterministic): the citation must resolve to a real
   section via `chunks(act_short, section_id)`. Kills invented citations.
2. **Quantity consistency** (deterministic): every (value, unit) the model
   introduced — i.e. not echoed from the user's question — must appear in
   the cited section. Kills "7 days" cited against a 14-day rule, "21 days
   leave", wrong wage figures. Question-echoed quantities are exempt: good
   answers restate the user's situation.
3. **Semantic support** (statistical, weakest): claim-vs-source bge-small
   cosine. Calibration (2026-07-02, real pairs): true 0.699-0.822, false
   0.552-0.628, band-mismap 0.680 — inside the true range. Thresholds
   pass≥0.66 / flag≥0.55; this layer is only trusted to reject wrong-topic
   citations.

**Honest limitation (goes in REPORT.md):** a claim that picks the WRONG
BAND from a graduated table (e.g. "4 years → one month's notice", where
"one month" genuinely appears in s.11) is invisible to all three layers.
Mitigation is architectural: the Week-4 intent router sends banded/tenure
computations to the deterministic rules engine, and the notice bands are in
its scope from day one.

**Verdicts:** verified / flagged (weak support — shown with warning) /
failed (unresolvable citation or ungrounded quantity — stripped and
regenerated once with the correct section injected).

## ADR-008 — Embeddings: bge-small-en-v1.5 F16, summary-first embedding (2026-07-02)

**Decision:** `bge-small-en-v1.5` (33M params, 384-dim, 64 MB F16 GGUF, via
llama.cpp) over `nomic-embed-text` (137M, 768-dim, ~270 MB).

**Why:** 4× smaller RSS (fits the 0.15 GB budget with slack — every MB feeds
S_eff), ~4× faster on CPU, half the vector storage. Its 512-token context
would truncate long sections, but the SAC design already makes the summary
the retrieval surface: we embed `header + summary + leading text` (truncated
to ~1800 chars), and the FTS5 keyword leg searches the full text. Query
embedding uses BGE's required prefix ("Represent this sentence for searching
relevant passages: ..."); document embedding does not.

**Known limitation:** English-only. Pidgin degrades gracefully (shared
lexicon + FTS5 leg); Yoruba/Hausa/Igbo queries are handled by Week-6 query
understanding (translate → retrieve), not by the embedder.

**Revisit trigger:** golden-set retrieval recall (Week 3 harness) below
target with failures attributable to embedding quality → re-evaluate nomic
at Q8 before touching chunking.

## ADR-007 — Certification on RAM-capped VM, not refurb hardware (2026-07-01)

**Decision (Ola):** No refurb laptop purchase. Performance/thermal
certification runs happen on an 8 GB RAM-capped VM using adtc-profiler,
mirroring the audit environment.

**Supporting fact:** the official audit itself runs in cloud VMs
(profiler README: "secure cloud VMs", Docker `--memory=7.5g`). A capped VM
is closer to audit conditions than arbitrary refurb hardware; the ±15% RSS /
±25% TPS participant-vs-audit tolerances favour environment parity.

**Costs accepted:**
- Thermal evidence is weaker (VMs don't expose real core temps; the -10
  thermal penalty is judged on the audit side anyway).
- The Week-6 "budget laptop evidence pack" reframes from photos-of-refurb to
  VM-profile benchmark runs. `budget_laptop_claim: true` stays (mandatory
  for all submissions per template).

**Eligibility note resolved:** entry via Ruach Tech's Nigeria-HQ company
(incorporated Lagos 2025) — satisfies the residency/African-country
requirement and the <12-months venture rule. DevPost registration done.

## ADR-006 — Base model locked: Llama 3.2 3B Instruct Q4_K_M (2026-07-01)

**Decision:** Lock **Llama 3.2 3B Instruct (Q4_K_M, bartowski GGUF)** as the
base model. Runner-up: Phi-3.5-mini. Eliminated: Qwen 2.5 3B.

**Evidence:** 20-question bake-off, 3 models, scored per
`eval/bakeoff/RUBRIC.md` (full scoresheet: `eval/bakeoff/SCORES.md`).
Totals: Llama 25/40, Phi 22/40, Qwen 21/40. But the totals matter less than
*which* weaknesses each model has and whether our architecture can engineer
around them:

| Weakness | Fixable by engineering? |
|---|---|
| Llama: misread one notice band (G01); shoehorned one out-of-scope extraction (X04) | **Yes** — banded computations move to the deterministic rules engine (Pillar 2); the verifier's claim-vs-source similarity check catches band misreads; "unknown" few-shots fix X04 |
| Phi: markdown-fenced JSON, grammar non-compliance, verbosity | Partly — GBNF forces structure, but verbosity costs latency on 4 vCPU and its 3.8B size costs ~20% TPS + 300 MB vs Llama |
| Phi/Qwen: Pidgin register wrong (Phi: generic broken English; Qwen: complete generation collapse into gibberish/token loops) | **No** — language priors at 3B are not fixable by prompting. Pidgin is Pillar 3, worth +15% African Alpha and the localisation award |
| Qwen: confident fabrication closed-book (invented "21 days, s.23, Act 2007"; invented ₦5M court rule) | Dangerous — the standalone-GGUF audit and LM Studio judge test run without our retrieval scaffold |

**Deciding factors, in order:**
1. Llama's Pidgin was decisively the most natural and legally accurate —
   the only unfixable differentiator in the field.
2. Llama's weaknesses land exactly where Pillars 1–2 already provide
   deterministic backstops.
3. Smallest file (1.9 GB) and second-fastest (48 t/s dev machine) — best
   S_eff/S_perf posture after Qwen, without Qwen's fabrication risk.
4. Llama 3.2 1B Instruct shares the tokenizer → speculative-decoding draft
   option preserved (Week 5, demo UX only per ADR-003).

**License note:** Llama 3.2 Community License — redistribution allowed with
attribution ("Built with Llama") and the license text. Add both to README and
the final HF model repo in Week 7 packaging.

**Revisit trigger:** if Week 3 citation-precision numbers on the golden set
come in under target and error analysis attributes it to the base model (not
retrieval), re-run this bake-off including Phi with GBNF enforcement before
the Week 4 corpus expansion.

## ADR-005 — Name "Òfin" cleared for use (2026-07-01)

**Decision:** Keep the working name **Òfin** (Yoruba: "law").

**Evidence:** Web search across Nigerian legal-tech landscape (StartupList
Africa, Tracxn legal-tech Nigeria, Legal Tech Africa directories) found no
product or company named "Ofin"/"Òfin". Existing players occupy different
names: LawPavilion, PocketLawyers, DIYlaw, Modulaw, VESTI, JUDY, Sidebrief —
and the plan already avoids LawPadi/SabiLaw. No trademark red flags surfaced
in ordinary search.

**Caveat:** This is a collision check, not legal clearance. A proper NIPO
(Nigerian trademark registry) search is a post-contest task if the product
commercialises.

## ADR-004 — Bake-off model downloads use bartowski community quants (2026-07-01)

**Decision:** Source all three bake-off GGUFs (Qwen 2.5 3B Instruct, Llama 3.2
3B Instruct, Phi-3.5-mini) from `bartowski/*` Hugging Face repos, Q4_K_M.

**Why:**
- Meta's official Llama 3.2 HF repo is license-gated (requires an authenticated
  account that has accepted the license). ADTC rules require `download_model.sh`
  to work **without any credentials**, so gated repos are unusable for
  submission and we may as well develop against ungated ones.
- bartowski uses a consistent imatrix quantization recipe across all three
  models, which keeps the bake-off comparison fair (same quant method, only the
  base model varies).

**Licensing note for the final pick:** Qwen 2.5 is Apache-2.0, Phi-3.5 is MIT,
Llama 3.2 is Meta Community License (redistribution allowed with attribution,
but adds friction). All else equal, licensing favours Qwen or Phi.

## ADR-003 — Profiler-derived scoring strategy (2026-07-01)

**Context:** Read `adtc-profiler` source/README and the submission template.
Three facts constrain strategy:

1. **The automated audit profiles the raw GGUF via `llama-bench`**, not our
   application. S_perf (30%) and S_eff (20%) are measured on the bare model
   file named in `metadata.json`. The verifier / rules engine / UI earn their
   keep via S_acc (50%), REPORT.md, and human judging — not via the perf audit.
2. **`TPS_REFERENCE = 15.0`** — S_perf = `min(TPS/15, 1) × 100`. Throughput
   above 15 tok/s on the audit VM (4 vCPU, 8 GB, iGPU) earns nothing extra.
   Speculative decoding is therefore a demo-UX optimisation, not a score lever,
   *unless* base TPS on the audit VM lands below 15.
3. **S_eff = `max(0, (7.0 − peak_RSS_GB)/7.0) × 100`** — a ~2.5 GB peak RSS 3B
   model scores ~64. Each 0.7 GB saved ≈ +10 S_eff ≈ +2 total points.
   Accuracy's 50% weight dominates: a 3B that answers legal questions
   correctly beats a 1B that saves 1.5 GB.

**Decision:** Optimise in this order: (1) accuracy via retrieval + verifier,
(2) keep peak RSS lean but do not sacrifice model size for it, (3) confirm
base TPS ≥ 15 on target hardware early — if it is, stop optimising throughput.

## ADR-002 — Dev machine vs certification machine (2026-07-01)

**Context:** Development happens on a MacBook (Apple Silicon, 64 GB RAM). The
challenge targets a 4 vCPU / 8 GB / integrated-GPU laptop profile, and the
plan claims the +10% budget-laptop bonus on refurb-class hardware.

**Decision:** All *quality* evaluation (bake-off, golden set) runs on the Mac —
model outputs are hardware-independent. All *performance* numbers recorded on
the Mac are labelled `dev-baseline (M-series Mac)` and never quoted in
REPORT.md as target-hardware results. A real 8 GB x86 machine must be acquired
in Week 1–2 (owner: Ola); profiler certification runs happen there.

**Risk if ignored:** Mac TPS numbers are 3–10× target hardware; building the
perf story on them would collapse at audit time (±25% TPS tolerance between
participant-reported and audit numbers).

## ADR-001 — Repo layout mirrors the official submission template (2026-07-01)

**Decision:** `metadata.json`, `download_model.sh`, `REPORT.md`, `model/` live
at the repo root exactly as in `adtc-2026-submission-template`, from day one.
Application code lives in `engine/`, corpora in `corpus/`, evaluation in
`eval/`.

**Why:** The judges' evaluation framework runs mechanically against this
structure ("must run without errors"). Restructuring in Week 7 is scheduled,
but starting compatible means Week 7 is a diff review, not a migration.
