# Òfin — Receipt-Backed Offline Legal Companion for Nigeria

> **Òfin** (Yoruba: *law*) is an on-device assistant for Nigerian labour, tenancy, and tax
> law that runs fully offline on an 8 GB laptop. It never asks you to trust it —
> **every answer comes with receipts.**

Built for the [Africa Deep Tech Challenge 2026](https://adtc-2026.devpost.com)
(Laptop LLM track, `corporate_enterprise` domain) by Ruach Tech.

## The three pillars

1. **Verified Citation Engine** — the model must cite every legal claim
   (`[Labour Act 2004, s.11(2)]`). A deterministic verifier checks each citation
   against the local statutory corpus; unverified claims are stripped or flagged
   before the user sees them.
2. **Deterministic Computation Engine** — PAYE tax, severance, and notice-period
   calculations are pure code implementing the statutes. The LLM extracts
   parameters and narrates results; it never does arithmetic.
3. **Naija-Native Interaction** — Nigerian Pidgin as a first-class interaction
   mode, with Yoruba/Hausa/Igbo query understanding.

## Repository layout

```
ofin/
├── metadata.json        # ADTC submission metadata (template-compatible)
├── download_model.sh    # Downloads the .gguf model (template-compatible)
├── REPORT.md            # Technical writeup (template-compatible)
├── model/               # GGUF lands here; never committed
├── corpus/              # Statutory texts as clean markdown + provenance
├── engine/              # Go: verifier, rules engine, retrieval, web UI
├── eval/                # Bake-off + golden evaluation sets and harness
├── scripts/             # Build-time tooling (corpus prep, SAC enrichment)
└── docs/                # Decision log, architecture, weekly progress
```

## Quick start (development)

```bash
# 1. Get the model
bash download_model.sh

# 2. Smoke-test with llama.cpp (installed via `brew install llama.cpp`)
llama-cli -m model/*.gguf -p "What notice period does the Nigerian Labour Act require?" -n 256
```

Full application quick-start lands in Week 2 (retrieval CLI) and Week 5 (web UI).

## Status

Week 1 (July 1–7, 2026): foundations — model bake-off, corpus sourcing,
baseline profiling. See [docs/PROGRESS.md](docs/PROGRESS.md).

## Legal disclaimer

Òfin provides general legal information derived from published Nigerian
statutes, with citations to the source text. It is not legal advice and is not
a substitute for a qualified legal practitioner.
