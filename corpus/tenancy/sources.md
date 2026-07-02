# Tenancy Corpus — Sources and Provenance

As-at date: **July 2026**. Tenancy is STATE law — everything here is Lagos
State only, and answers must flag that jurisdiction (metadata carries
`jurisdiction: lagos-state`).

| File | Law | Source | Cleaning | Verification |
|---|---|---|---|---|
| `tenancy-law-lagos-2011.md` | Tenancy Law of Lagos State, No. 14 of 2011 | [Lagos Ministry of Justice](http://lagosministryofjustice.org/wp-content/uploads/2022/01/Tenancy-Law-2011.pdf) — official Lagos State Gazette No. 37 Vol. 44 (26 Aug 2011) | `pipeline/convert_gazette.py` | 48/49 sections chunked; key provisions (s.1 application incl. the Apapa/Ikeja GRA/Ikoyi/VI exemptions, s.13 notice lengths) read and verified during golden-set drafting |

Known issues: three-column Arrangement of Sections bleeds into a few
harvested titles (s.4, s.6, s.28 — QA-flagged); s.15 title missing.
