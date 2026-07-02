# hardening.jsonl — section verification notes

Every (act, section) pair below was verified against the chunk text in
`data/chunks-sac/*.json` (matched on `metadata.act_short` + `metadata.section_id`)
on 2026-07-02. Quote fragments are verbatim from the chunk text.

- **H01** — `labour-act-cap-l1-lfn-2004.json` s.11: "one month, where the contract has continued" · `nigeria-tax-act-2025.json` s.50: "compensation for loss of office or employment"
- **H02** — `nigeria-tax-act-2025.json` s.4: "rents or interests arising from a right" · `tenancy-law-lagos-2011.json` s.4: "rent in excess of one (1) year"
- **H03** — `national-minimum-wage-act-2019-consolidated.json` s.3: "not less than N70,000.00 per month" · `labour-act-cap-l1-lfn-2004.json` s.15: "intervals not exceeding one month"
- **H04** — `labour-act-cap-l1-lfn-2004.json` s.54: "not less than fifty per cent of the wages" · `nigeria-tax-act-2025.json` s.4: "salaries, wages, fees, allowances, compensations"
- **H05** — `nigeria-tax-act-2025.json` s.186: "land or building including interest in land" · `tenancy-law-lagos-2011.json` s.37: "increase in rent payable ... is unreasonable"
- **H06** — `labour-act-cap-l1-lfn-2004.json` s.5: "contributions to provident or pension funds" · `nigeria-tax-act-2025.json` s.30: "contributions under the Pension Reform Act"
- **H07** — `nigeria-tax-act-2025.json` s.30: "rent relief of 20% of annual rent paid"
- **H08** — `nigeria-tax-act-2025.json` s.163: "gross income of national minimum wage or less"
- **H09** — `nigeria-tax-act-2025.json` s.56: "a small company, at 0%"
- **H10** — `nigeria-tax-act-2025.json` s.187: "all medical and pharmaceutical products including"
- **H11** — `nigeria-tax-act-2025.json` s.153: "furnish the purchaser with a VAT invoice"
- **H12** — `labour-act-cap-l1-lfn-2004.json` s.11: "two weeks, where the contract has continued"
- **H13** — `labour-act-cap-l1-lfn-2004.json` s.11: "period of two years but less than five"
- **H14** — `nigeria-tax-act-2025.json` s.58: "as specified in the Fourth Schedule" · sch.7: "First N800,000 at 0%" (statutory Fourth Schedule is chunked as `sch.7`)
- **H15** — `nigeria-tax-act-2025.json` s.58: "other than an individual earning the Minimum Wage" · s.163: "gross income of national minimum wage or less"
- **H16** — `tenancy-law-lagos-2011.json` s.4: "new or would be tenant rent in excess" · s.5: "obliged to issue a rent payment receipt"
- **H17** — `labour-act-cap-l1-lfn-2004.json` s.18: "at least six working days" · s.19: "excluding overtime and other allowances"
- **H18** — `nigeria-tax-act-2025.json` s.20: "rent and premiums ... land or building occupied" · s.21: "domestic or private expense"
- **H19** — `tenancy-law-lagos-2011.json` s.8: "Effect repairs and maintain the external"
- **H20** — `nigeria-tax-act-2025.json` s.187: "basic food items"
- **H21** — negative: no sections (matrimonial causes law not in corpus — refusal expected)
- **H22** — negative: no sections (criminal procedure not in corpus — refusal expected)

## Corpus surprises found while authoring (do not silently rely on these)

1. **NTA 2025 schedule IDs do not match statutory schedule names.** The chunker
   numbered schedules by position: the statutory *Fourth Schedule* (individual
   income tax bands, "Section 58(1)") has `section_id: sch.7`, while `sch.4` is
   actually the *Second Schedule* (export processing zones, "Section 60").
   Any eval or verifier logic mapping "Fourth Schedule" -> `sch.4` will be wrong.
2. **ECA 2010 chunks are title-only stubs** (13–114 chars, e.g. s.4 is just
   "4. Employee's notification of injury"). No new questions cite ECA; existing
   E01–E08 expected answers cannot be supported by current corpus text.
3. **No PAYE machinery in NTA 2025** — "Pay As You Earn" appears nowhere in the
   chunk text (grep hits were substrings of "payer/payable"). Employer monthly
   deduction/remittance obligations live in the Nigeria Tax Administration Act
   2025, which is not in the corpus.
