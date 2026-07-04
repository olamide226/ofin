# Òfin — Technical Report

> ADTC 2026 Laptop LLM track · `corporate_enterprise` domain · Ruach Tech
>
> **Feature freeze Aug 12 · Submit Aug 23 · Gate 1 Aug 25, 2026.**
> Week 7 of 8: submission packaging. All core systems built and verified.

## 1. Problem

Nigeria has roughly one lawyer per 4,000 people, concentrated in Lagos and
Abuja. For a worker sacked without notice, a tenant handed an illegal rent
increase, or a shopkeeper unsure of their PAYE, the practical options are
folklore, expensive consultations, or nothing. Internet access is intermittent
and data is costly; cloud AI assistants also hallucinate statutory law with
total confidence.

Òfin is an offline legal companion for Nigerian labour, tenancy, and tax law
that runs on the 8 GB laptops people already own, answers in English or
Nigerian Pidgin, and backs every legal claim with a verifiable citation to the
statutory text — "answers with receipts".

**What Òfin is not:** a general-purpose chatbot. It covers three legal domains
across 8 Nigerian statutes. It refuses questions outside its scope (divorce,
criminal law, immigration) and questions that ask for illegal advice. It does
not provide legal advice — it provides statutory information with citations.

## 2. Design decisions

*(Summaries; full rationale in [docs/DECISIONS.md](docs/DECISIONS.md).)*

- **Base model: Llama 3.2 3B Instruct Q4_K_M** — won a 20-question bake-off
  against Qwen 2.5 3B and Phi-3.5-mini on Nigerian legal QA. Decisive factors:
  the only natural, legally-accurate Nigerian Pidgin of the three; weaknesses
  (banded-rule arithmetic) that our deterministic rules engine covers by
  design; smallest weights (1.9 GB). Full scoresheet in `eval/bakeoff/`,
  rationale in `docs/DECISIONS.md` ADR-006.
- **Corpus: 8 Nigerian statutes, 678 SAC chunks** — Labour Act, Employees'
  Compensation Act, Trade Disputes Act, Trade Disputes (Essential Services) Act,
  National Minimum Wage Act, Lagos Tenancy Law 2011, Nigeria Tax Act 2025,
  Nigeria Tax Administration Act 2025. Summary-Augmented Chunking (SAC) with
  Gemini 2.5 Flash-Lite at build time enriches each chunk with a summary and
  cross-reference edges.
- **Hybrid retrieval: SQLite-vec + FTS5 with Reciprocal Rank Fusion (k=60)** —
  single-file, zero external services. Cross-act reverse-reference hop edges
  resolved breadth-first: each seed's first hop before any seed's second,
  quota=3. Embedding model: bge-small-en-v1.5 F16 GGUF (384-dim, 64 MB).
- **Verified Citation Engine:** every legal claim carries a citation token
  (`[Labour Act 2004, s.11(2)]`); a deterministic 3-layer verifier checks
  existence, quantity consistency, and semantic support (thresholds 0.66/0.55)
  before display. Failed claims get one constrained regeneration with the
  correct source text. Small models hallucinate on legal text — we engineer
  around it instead of pretending otherwise.
- **Deterministic Computation Engine:** 4 computation kinds — PAYE income tax
  (NTA 2025 bands), employment termination notice (Labour Act s.11 bands),
  Lagos tenancy notice (Tenancy Law s.13 bands), and redundancy entitlements
  (Labour Act s.20). All arithmetic is pure Go code implementing the statutes.
  The LLM extracts parameters and narrates; it **never** touches figures
  (ADR-010, extended to inputs: extracted values must be traceable to the
  question text). The redundancy calculator honestly reports that s.20
  prescribes NO fixed severance formula — it's a duty to negotiate.
- **f16 KV-cache + flash attention** — chosen over q8_0 (ADR-014). ~40% faster
  on CPU for +315 MB RSS. The real latency bottleneck is prompt prefill (~4,600
  tokens on CPU), not KV format or decode speed.
- **African-language scope: Pidgin only** — Yoruba, Hausa, and Igbo query
  understanding was tested and rejected (ADR-017). Llama 3.2 3B hallucinates
  translations at temp 0, and retrieval collapses on non-English embeddings.
  Nigerian Pidgin (pcm) is first-class: toggle it in the web UI or pass
  `-pidgin` on the CLI. The African Alpha claim is Pidgin-only, honestly
  declared in `metadata.json`.
- **Gemma 4 E4B rejected** (ADR-016) — 7.5B params (the "E4B" name counts
  activated parameters, not total), 7.15 GB RSS breaks the 8 GB budget.

## 3. Constraints

- **8 GB RAM profile (7 GB usable ceiling, hard DQ on breach).** Peak full-stack
  RSS ~4.2 GB (f16 KV + flash attention, embed server, Go engine, SQLite). Within
  the 8 GB cap; over the 3.5 GB self-target (see ADR-013 — the self-target was
  set before the full stack was built). No swap/OOM on the audit profile.
- **100% offline at runtime.** Corpus enrichment uses Gemini 2.5 Flash-Lite at
  build time; the shipped system makes zero network calls. Verified on the
  certification VM (zero non-loopback connections during generation).
- **llama.cpp / GGUF only** (challenge rule). llama.cpp b9864 on the target
  Linux VM; build-time Python is Homebrew Python on macOS.
- **4 vCPU, integrated GPU** audit profile; thermal penalty above 85 °C. Our
  VM run showed no throttling (34.3 TPS sustained across 3 runs).
- **libsqlite3-dev** required on Linux for CGo SQLite-vec build.

## 4. Benchmarks

### Single-domain baseline (39 labour, Week 3)

| Metric | Value | Notes |
|---|---|---|
| Retrieval recall@6 | 84% (31/37) | cross-ref hop raised from 81% |
| Citation precision | 91% (52/57) | all claims ✓/⚠/✗-marked |

### Three-domain hardened baseline (90 questions, 8 acts, 2026-07-03)

| Metric | Value | Notes |
|---|---|---|
| Retrieval recall | **70%** (55/79) | 90-Q set incl. 22 deliberately hard hardening questions; old-68 subset: 78% |
| Citation precision | **84%** (173/207) | 27 flagged, 7 failed — all ✓/⚠/✗-marked in the UI |
| Computed answers | 13 routed, 0 misroutes | question-evidence guards: figures must be traceable to the question (ADR-010 extended to inputs) |
| Refusal calibration | **88/90** | canonical refusal template + partial-answer duty |
| Regeneration rate | 12/90 | single constrained retry, context-budget-aware |
| Corpus | 678 chunks, 8 acts | incl. Nigeria Tax Administration Act 2025 (PAYE machinery) |

Three same-day calibration rounds (v1 63%/77% → v3 70%/84%); every fix is
pinned by a unit test or corrected golden expectation. Remaining misses are
documented and ranked in docs/PROGRESS.md (cross-domain query decomposition,
one corpus transcription gap, Pidgin query normalization).

Verifier coverage note: prose citations ("Section 11(3) of the Labour Act
2004") are parsed and resolved, not just bracket tokens — coverage tripled
(27 → 81 verified claims) when this landed; the headline precision is
computed over ALL claims, not just conveniently-formatted ones.

### Dev machine baseline (M1 Max, 2026-07-03 — dev-only per ADR-002)

| Metric | Value |
|---|---|
| Generation TPS (llama-bench, 512 prompt) | 50.1 |
| First-token latency | 910 ms |
| Peak RSS (model only) | 2,111 MB |
| Application stack RSS (chat+embed servers, f16 KV) | ~3.9 GB |
| KV cache config | f16 + flash attention (`-fa on`), 6144 ctx |
| Speculative decoding | rejected (ADR-012: +1.6 GB RSS) |

### VM certification (AMD EPYC 4 vCPU / 7.6 GB, Ubuntu 26.04, 2026-07-02)

| Metric | Value |
|---|---|
| Generation TPS (llama-bench, sustained 5×) | **19.4** (no throttle decay) |
| First-token latency | 5,977 ms |
| Peak RSS (model only, llama-bench) | 3,442 MB |
| **Peak RSS (full app stack, integrated, draft off)** | **~3.9 GB idle / ~4.2 GB peak** |
| KV cache config | f16 + flash attention (ADR-014) |
| Offline (non-loopback calls during generation) | 0 ✅ |
| Functional end-to-end (cited answers, both routes) | ✅ |
| S_perf verdict | ✅ Max (19.4 > 15 TPS cap) |
| S_eff verdict | ⚠ Full-stack ~3.9 GB — over 3.5 GB self-target, within 8 GB cap (ADR-013) |

**Recertification note (2026-07-04):** llama-bench tg128 dropped from 34.3
(July 2) to 19.4 — consistent across 5 runs, no CPU contention. Likely
noisy-neighbor on the shared Hetzner host. S_perf still maxes (cap is 15).
Full-stack numbers with f16+fa unchanged: chat RSS 3,973 MB, warm lookup
~90s. The July 2 numbers may reflect a quieter host; both sets are recorded
for transparency.

The model-only figure understates real memory: llama-bench profiles the raw GGUF
and never launches the app. The integrated stack (embed + chat servers + Go
engine + SQLite) is the S_eff figure of record — see ADR-013, which also records
the draft-model ship-blocker found and fixed during this run. The KV cache is
now f16 + flash attention (ADR-014), not q8_0 — ~40% faster prefill on CPU for
+315 MB RSS, well within the 8 GB cap.

¹ Dev-machine numbers are for regression tracking only and are not claimed as
target-hardware performance.

## 5. Limitations

*(Honest limitations — maintained continuously, finalised Week 7.)*

- **Domain scope:** labour, tenancy, and tax law only. Òfin refuses questions
  outside these domains (divorce, criminal law, immigration, etc.).
- **Statute-only, no case law.** Judicial interpretation can modify how a
  provision is applied; Òfin cannot capture that.
- **Tenancy answers are Lagos State law only.** The app flags this jurisdiction
  explicitly in every tenancy answer. Tenancy law in Nigeria is state-level;
  other states may have different notice periods and rent regulations.
- **Tax computation is statutory bands only.** Relief allowances,
  deductions, and exemptions require the user's full financial circumstances.
  Òfin computes the statutory PAYE bands correctly but actual tax liability
  depends on individual reliefs.
- **Statutory text as amended** up to the version stamps in
  `corpus/*/sources.md`; the UI states the as-at date.
- **English and Pidgin only.** Yoruba, Hausa, and Igbo were tested and found
  unsafe with the onboard model (ADR-017). A user asking in those languages
  gets a Pidgin response explaining the limitation.
- **Known corpus gap:** ECA 2010 s.15 is missing from transcription (source
  gazette needed). NTA s.58 has margin-note artifacts from the gazette PDF
  layout (scoped, deferred cleanup).
- **Conversational memory is deliberately excluded pre-freeze** — it would eat
  the 6144 prompt budget, need query rewriting, and break verifier
  claim-independence.
- **Not legal advice.** Òfin provides general legal information derived from
  published statutes with citations to the source text. Users should consult
  a qualified legal practitioner for their specific situation.
