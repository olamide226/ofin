# Labour Corpus — Sources and Provenance

Corpus policy: every act records where its text came from, what cleaning was
applied, and its verification status. The Verified Citation Engine's receipts
are only as trustworthy as this table. As-at date: **July 2026**.

| File | Act | Source | Cleaning applied | Verification status |
|---|---|---|---|---|
| `labour-act-cap-l1-lfn-2004.md` | Labour Act, Cap L1 LFN 2004 | [mykeels/nigerian-laws](https://github.com/mykeels/nigerian-laws) (community transcription) | `scripts/clean_corpus.py` (105 run-together-word fixes, log reviewed) | ⚠️ Pending section-by-section cross-check vs official PLAC PDF (`raw/labour-act-L1-plac.pdf`) — Week 2 |
| `employees-compensation-act-2010.md` | Employees' Compensation Act, 2010 | [mykeels/nigerian-laws](https://github.com/mykeels/nigerian-laws) | `scripts/clean_corpus.py` (1 fix) | ⚠️ Pending cross-check vs official gazette — Week 2 |
| `trade-disputes-act-cap-t8-lfn-2004.md` | Trade Disputes Act, Cap T8 LFN 2004 | [PLAC lawsofnigeria.placng.org/laws/T8.pdf](https://lawsofnigeria.placng.org/laws/T8.pdf) (official publisher) | `pdftotext -layout` + page-header strip | ⚠️ Raw conversion; needs structural markdown pass (headings per section) — Week 2 |
| `trade-disputes-essential-services-act-cap-t9-lfn-2004.md` | Trade Disputes (Essential Services) Act, Cap T9 LFN 2004 | [mykeels/nigerian-laws](https://github.com/mykeels/nigerian-laws) | `scripts/clean_corpus.py` (1 fix) | ⚠️ Pending cross-check — Week 2 |
| `national-minimum-wage-act-2019-consolidated.md` | National Minimum Wage Act 2019, as amended by NMW (Amendment) Act 2024 | Official Gazette No. 62 Vol. 106 (23 Apr 2019) via [gazettes.africa](https://archive.gazettes.africa/archive/ng/2019/ng-government-gazette-supplement-dated-2019-04-23-no-62.pdf); amendment per [PLAC bill text](https://placng.org/i/wp-content/uploads/2024/07/National-Minimum-Wage-Amendment-Bill-2024.pdf) | Hand-transcribed from gazette OCR, 2024 amendment applied inline with `[Amended 2024]` markers | ✅ Hand-verified against gazette during transcription |

## Raw artifacts (`raw/`, gitignored)

- `labour-act-L1-plac.pdf` — official PLAC print of Labour Act (cross-check authority)
- `trade-disputes-act-T8-plac.pdf` + `trade-disputes-T8.txt` — official PLAC print + extraction
- `nmw-2019-gazette.pdf` + `nmw-2019-gazette.txt` — official gazette (scanned, OCR layer)
- `nmw-act-2019-natlex.pdf` — ILO NATLEX copy (image-only, no text layer; kept for reference)
- `nmw-amendment-2024-bill-plac.pdf` + `nmw-2024.txt` — 2024 amendment bill text

## Corpus interventions log (Week 2)

| Date | File | Intervention | Authority |
|---|---|---|---|
| 2026-07-01 | `labour-act-…md` | **s.80 (Jurisdiction) restored** — missing from community transcription | Official PLAC PDF `raw/labour-act-L1-plac.pdf` p.~2366 |
| 2026-07-01 | `trade-disputes-act-…md` | **National Industrial Court Rules split out** to `national-industrial-court-rules.md` (subsidiary legislation; its own rule numbering broke section parsing). Note: TDA ss.20–32 repealed by NIC Act 2006 No. 37 — the numbering gap is correct | PLAC PDF itself marks the repeal |
| 2026-07-01 | `trade-disputes-essential-services-…md` | **ss.1, 2, 7 section numbers reconstructed** (number lines lost in transcription; bodies were bare "(1)…"). Marked inline with HTML comments | ⚠️ Unverified — PLAC has no T9.pdf (404); verify against LFN 2004 print when found |
| — | `employees-compensation-act-2010.md` | **s.15 missing** from transcription (numbering jumps 14→16) | ⚠️ Open gap — need official ECA gazette to restore |

## Known corpus gotchas (learned Week 1)

1. **PLAC `print.php?sn=323` serves the repealed Cap N61 NMW Act (₦5,500).**
   Nigerian legal websites frequently serve superseded versions without
   warning. Always confirm the repeal chain: Cap N61 → NMW Act 2019 (₦30,000)
   → NMW (Amendment) Act 2024 (₦70,000, 3-year review).
2. Community transcriptions have a systematic run-together-word artifact at
   list-item boundaries; `scripts/clean_corpus.py` fixes it conservatively
   (dictionary-gated). Always review the fix log — the macOS word list lacks
   British spellings and irregular verb forms, which caused false positives
   until they were added to the script's supplemental dictionary.
3. ILO NATLEX PDFs may be image-only scans (no text layer).
