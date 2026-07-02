# Tax Corpus — Sources and Provenance

As-at date: **July 2026**. The Nigeria Tax Act 2025 (in force 1 January
2026) **consolidated and repealed** PITA, CITA, the VAT Act and the Capital
Gains Tax Act — the plan's original PITA/CITA/VAT sourcing list is obsolete;
this one Act covers the tax domain.

| File | Act | Source | Cleaning | Verification |
|---|---|---|---|---|
| `nigeria-tax-act-2025.md` | Nigeria Tax Act, 2025 (Act No. 7) | Official Gazette No. 117 Vol. 112 (26 Jun 2025) via the [Gambia Revenue Service mirror](https://irs.gm.gov.ng/docs/national/NIGERIA_TAX_ACT_2025.pdf) — the [NRS original](https://www.nrs.gov.ng/uploads/NIGERIA_TAX_ACT_2025_ef6bb812a5.pdf) and CITN copies are image-only scans with unusable OCR; the mirror carries a clean text layer of the same gazette | `pipeline/convert_gazette.py` (page-furniture strip only) | ✅ **PAYE rate bands cross-checked 2026-07-02**: gazette Fourth Schedule text (under s.58(1)) matches the rules engine exactly. 202/203 sections chunked |

Raw artifacts in `raw/` (gitignored): gm-mirror PDF (canonical), NRS + CITN
scans (kept for reference), `nta-2025.txt` extraction.

Known layout gotchas: left- AND right-margin marginal titles; two-column
Arrangement of Sections; margin notes wrap into bare "Schedule" lines
(chunker's `is_schedule_heading` exists because of this).
