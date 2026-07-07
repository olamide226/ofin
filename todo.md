# Òfin — remaining work (Week 7 → submission)

## Done (July 4–6 session)

- [x] Eval harness fixes — computation Qs excluded, partial-answer refusal detection
- [x] VM re-certification with f16+fa (19.4 t/s, 3,973 MB chat RSS)
- [x] Evidence screenshots (7 shots) + demo video (3 min)
- [x] GGUF template baked + uploaded to HF (`olamide226/ofin-model`, public)
- [x] download_model.sh → baked model
- [x] REPORT.md polished
- [x] **NTA s.58 margin-note cleanup** — 773 lines stripped, 134 chunks cleaned. TX03 now passes (was refusing since Week 4)
- [x] Descriptive retrieval prefixes for s.33, s.58, s.186, s.187, sch.7, s.4
- [x] **Query decomposition infrastructure** — splits compound questions, batch-embeds sub-queries, merges results. Limited by domain-conflicting vocabulary (e.g. "rent" activates tenancy even in tax sub-queries). Needs query rewriting layer.

## What's left

### Query rewriting for cross-domain (makes decomposition effective)

- [ ] Add domain-specific terms to each sub-query after splitting. e.g. "Do I pay income tax on the rent I collect" → append "Nigeria Tax Act chargeable income". "Can I ask a new tenant for two years' rent upfront" → append "Lagos Tenancy Law advance rent". Simple keyword injection leveraging FTS5.

### Pidgin normalization

- [ ] Build ~30-term Pidgin→statutory-English glossary. Apply before embedding. Fixes P01, P12, P16, potentially N03.

### Remaining single-domain lookup misses

- [ ] H10/H20 — VAT exemptions s.187 now at rank 5. Close but needs ranking boost.
- [ ] TX08 — capital gains s.34 at rank 2, s.33 behind. Close.
- [ ] L12/L19 — wages/court jurisdiction
- [ ] E02/E07 — ECA gaps (s.5 missing from corpus)

### Post-freeze

- [ ] Cross-domain query rewriting (see above)
- [ ] Pidgin fine-tuning — needs 200-500 QA pairs + GPU
