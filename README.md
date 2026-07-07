# Òfin — Receipt-Backed Offline Legal Companion for Nigeria

> **Òfin** (Yoruba: *law*) is an on-device assistant for Nigerian labour,
> tenancy, and tax law that runs fully offline on an 8 GB laptop. It never asks
> you to trust it — **every answer comes with receipts.**

Built for the [Africa Deep Tech Challenge 2026](https://adtc-2026.devpost.com)
(Laptop LLM track, `corporate_enterprise` domain) by Ruach Tech.

## The three pillars

1. **Verified Citation Engine** — the model must cite every legal claim
   (`[Labour Act 2004, s.11(2)]`). A deterministic verifier checks each citation
   against the local statutory corpus (8 Nigerian acts, 678 chunks) using 3-layer
   semantic validation; unverified claims are stripped or flagged before the user
   sees them. Failed claims get one constrained regeneration with the correct
   source text.
2. **Deterministic Computation Engine** — PAYE tax bands, employment notice
   periods, Lagos tenancy notice bands, and redundancy entitlements are pure Go
   code implementing the statutes. The LLM extracts parameters and narrates
   results; it **never** touches figures. 4 computation kinds, 0 arithmetic
   hallucinations.
3. **Naija-Native Interaction** — Nigerian Pidgin (pcm) as a first-class
   interaction mode. Toggle it in the UI or pass `-pidgin` on the CLI. Yoruba,
   Hausa, and Igbo query understanding was tested and scoped out — Llama 3.2 3B
   cannot translate them safely at temp 0 (ADR-017). The African-language claim is
   Pidgin-only, honestly declared in `metadata.json`.

## System requirements (the 8 GB laptop target)

| Component | Requirement |
|---|---|
| RAM | 8 GB (peak full-stack RSS ~4.2 GB) |
| Disk | ~3 GB (model 1.9 GB GGUF + 2.8 MB SQLite DB) |
| CPU | 4 vCPU (x86 or ARM) |
| Network | **Zero** at runtime — fully offline |
| OS | Linux (target) or macOS (dev). **Windows: use WSL2** — see [docs/WINDOWS.md](docs/WINDOWS.md) |

Performance on the 4 vCPU / 7.6 GB audit profile: **34.3 TPS** generation
(S_perf capped at 15 — full marks), warm RAG+verify ~143s, offline check PASS.

## Quick start

### Prerequisites

- **Go 1.21+** with CGo enabled
- **llama.cpp** (commit b9864 or later — tested on Linux; Homebrew b9850 has a
  known embedding crash above 1700 chars on macOS, worked around in the ingest
  pipeline)
- **Python 3.11+** (build-time only — corpus pipeline)
- `libsqlite3-dev` on Linux (`brew install sqlite3` on macOS)
- `GOOGLE_API_KEY` (build-time only — Gemini 2.5 Flash-Lite for chunk
  enrichment. Set it once for the `make sac` step; never needed at runtime.)

### 1. Clone and get the model

```bash
git clone https://github.com/olamide226/ofin.git
cd ofin
bash download_model.sh          # → model/ofin-model.gguf (~1.9 GB)
```

### 2. Build the CLI

```bash
cd engine
go build -tags sqlite_fts5 -o bin/ofin ./cmd/ofin
cd ..
```

The `sqlite_fts5` build tag is **required** — this isn't optional. FTS5
powers the hybrid retrieval (vector + full-text search).

### 3. Ingest the corpus

```bash
make setup                     # create Python venv + install pipeline deps
make chunk                     # statute markdown → structured chunks
make sac                       # enrich chunks with summaries + cross-refs
make ingest                    # → data/ofin.db (SQLite-vec + FTS5)
```

`make sac` needs `GOOGLE_API_KEY` in your environment. This step enriches each
chunk with a summary and cross-reference edges. It runs once at build time;
the enriched chunks are what you ingest.

### 4. Ask a question

```bash
make ask Q="How much notice should my employer give me after 3 years of service?"
# or with Pidgin:
engine/bin/ofin -pidgin ask "My oga sack me without notice, I don work 4 years. Wetin I fit do?"
```

### 5. Launch the web UI

```bash
engine/bin/ofin serve           # → http://127.0.0.1:8090
```

The web UI is a single `go:embed` HTML file — no build toolchain, no npm, no
CDN. It works fully offline.

## Repository layout

```
ofin/
├── metadata.json             # ADTC submission metadata
├── download_model.sh         # Downloads the GGUF (template-compatible)
├── REPORT.md                 # Technical writeup
├── README.md                 # ← you are here
├── Makefile                  # Build targets: chunk, sac, ingest, ask, test
├── model/                    # GGUF lands here (gitignored)
├── corpus/                   # Cleaned statutes as markdown + sources.md
│   ├── labour/               # Labour Act, ECA, TDA, TDESA, NMW Act
│   ├── tenancy/              # Lagos Tenancy Law 2011
│   └── tax/                  # NTA 2025 + NTAA 2025
├── data/
│   ├── chunks/               # Structured chunks from chunk_statutes.py
│   ├── chunks-sac/           # SAC-enriched chunks (summary + cross_refs)
│   └── ofin.db               # SQLite-vec + FTS5 hybrid store (~2.8 MB)
├── engine/                   # Go runtime (verifier, rules engine, retrieval, web UI)
│   ├── cmd/ofin/main.go      # CLI entrypoint (ask / serve / stop)
│   ├── internal/
│   │   ├── app/              # Orchestration: App.Ask with typed Emitter callbacks
│   │   ├── answer/           # Prompt construction (system + user messages)
│   │   ├── retrieve/         # Hybrid retrieval (SQLite-vec + FTS5, RRF k=60)
│   │   ├── verify/           # 3-layer citation verifier + embedding cache
│   │   ├── router/           # Intent router → rules engine dispatch
│   │   ├── rules/            # Deterministic computation (PAYE, notice, tenancy, redundancy)
│   │   ├── llama/            # llama.cpp HTTP client (chat + embeddings)
│   │   └── web/              # go:embed single-file web UI + SSE handler
│   └── bin/                  # Compiled binary (gitignored)
├── pipeline/                 # Build-time Python (statute chunker, SAC enrichment, ingest)
├── eval/
│   ├── golden/               # Golden evaluation sets (90 + 25 Pidgin Qs)
│   │   ├── labour.jsonl      # 40 English labour Qs (Week 2)
│   │   ├── tenancy-tax.jsonl # 29 tenancy + tax Qs (Week 4)
│   │   ├── hardening.jsonl   # 22 hardening Qs (Week 6)
│   │   ├── pidgin.jsonl      # 25 Pidgin Qs (Week 7)
│   │   └── results/          # Archived evaluation runs
│   └── run_golden.py         # Evaluation harness
├── scripts/                  # bench_ask.sh, corpus tools
└── docs/                     # DECISIONS.md (ADRs), PROGRESS.md, benchmarks/
```

## Architecture at a glance

```
Question (English or Pidgin)
    │
    ▼
┌──────────────────────────────────────┐
│  Intent Router (LLM extraction)       │
│  Is this a computation or a lookup?   │
└──────┬──────────────────┬────────────┘
       │                  │
       ▼                  ▼
┌──────────────┐   ┌──────────────────┐
│ Rules Engine  │   │ Hybrid Retrieval │
│ (Go, pure)   │   │ (vec + FTS5, RRF)│
│ PAYE/notice/ │   │ 8 acts, 678 SAC  │
│ tenancy/     │   │ chunks + hop     │
│ redundancy   │   │ edges            │
└──────┬───────┘   └────────┬─────────┘
       │                    │
       │     ┌──────────────┘
       │     ▼
       │  ┌──────────────────┐
       │  │ LLM Draft         │
       │  │ (Llama 3.2 3B)   │
       │  │ Cited answer      │
       │  └────────┬─────────┘
       │           │
       │           ▼
       │  ┌──────────────────┐
       │  │ Citation Verifier │
       │  │ 3-layer: exist →  │
       │  │ quantity →        │
       │  │ semantic support  │
       │  └────────┬─────────┘
       │           │
       │           ▼
       │  ┌──────────────────┐
       │  │ Regeneration?     │
       │  │ Failed claims get │
       │  │ 1 constrained     │
       │  │ retry w/ correct  │
       │  │ source text       │
       │  └────────┬─────────┘
       │           │
       ▼           ▼
  ┌──────────────────────────┐
  │       Final Answer        │
  │  Deterministic figures +  │
  │  verified citations +     │
  │  receipt cards            │
  └──────────────────────────┘
```

## Evaluation

| Metric | 90-Q golden (English) | 25-Q Pidgin |
|---|---|---|
| Recall | 70% (old-68: 78%) | TBD |
| Citation precision | 84% | TBD |
| Refusal calibration | 88/90 | TBD |
| Computation misroutes | 0/13 | TBD |

Run the evaluation:
```bash
# English golden set (90 questions)
python3 eval/run_golden.py

# Pidgin eval set (25 questions)
python3 eval/run_golden.py --pidgin
```

## Key design decisions

See [docs/DECISIONS.md](docs/DECISIONS.md) for the full ADR log (ADR-001 through
ADR-017). Highlights:

- **Model: Llama 3.2 3B Instruct Q4_K_M** — bake-off winner, decisively best
  Pidgin (ADR-006)
- **Embeddings: bge-small-en-v1.5 F16 GGUF** — 384-dim, 64 MB, fast on CPU
  (ADR-008)
- **Hybrid retrieval: SQLite-vec + FTS5 with RRF k=60** — single-file, zero
  external services (ADR-008)
- **f16 KV-cache + flash attention** — ~40% faster than q8_0 on CPU, +315 MB
  RSS (ADR-014)
- **No speculative decoding** — 1B draft adds ~1.6 GB RSS, rejected for the 8
  GB budget (ADR-012)
- **No build toolchain for UI** — single `go:embed` HTML file with vanilla
  HTML/CSS/JS (ADR-011)
- **Tri-language (Yoruba/Hausa/Igbo) rejected** — Llama 3.2 3B hallucinates
  translations at temp 0 (ADR-017)
- **Gemma 4 E4B rejected** — 7.5B params, 7.15 GB RSS breaks the 8 GB budget
  (ADR-016)

## Status

**Week 7** (submission packaging). Feature freeze Aug 12, submit Aug 23, Gate 1
Aug 25, 2026.

See [docs/PROGRESS.md](docs/PROGRESS.md) for the detailed weekly log.

## License

Source code: MIT. The statutory texts in `corpus/` are reproductions of
Nigerian legislation and are not subject to copyright. The model weights are
subject to the Llama 3.2 Community License.

## Legal disclaimer

Òfin provides general legal information derived from published Nigerian
statutes, with citations to the source text. It is not legal advice and is not
a substitute for a qualified legal practitioner.
