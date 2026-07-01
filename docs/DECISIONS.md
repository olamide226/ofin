# Decision Log

Architecture Decision Records for Òfin. Newest first. Every entry states the
decision, the alternatives considered, and why. This log feeds REPORT.md
("Design Decisions") and Gate 2 Q&A prep.

---

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
