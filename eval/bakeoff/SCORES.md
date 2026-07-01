# Bake-off Scoresheet — July 1, 2026

Scored per RUBRIC.md against `expected` in questions.jsonl. Raw outputs in
`outputs/<model>/`. Scores are judgment calls; rationale noted where
non-obvious. Phi-3.5-mini section filled after its run completed.

## Qwen 2.5 3B Instruct vs Llama 3.2 3B Instruct

| Q | Qwen | Llama | Notes |
|---|---|---|---|
| G01 | 2 | 0 | **Llama misread the notice bands**: gave "one week / s.11(2)(b)" for a 3-year tenure (correct: two weeks, (c)). Qwen exact. |
| G02 | 0 | 0 | Both failed day arithmetic: "two weeks sick" = 10 working days, inside the 12-day cap; both claimed it exceeds. → rules engine must own day-counting. |
| G03 | 1 | 2 | Llama cited s.54(1)(a) and (c) precisely; Qwen lumped citation, missed the six-weeks-after prohibition. Both missed (b) explicitly. |
| G04 | 0 | 1 | **Qwen confidently wrong** ("not legal") — failed to apply the <25-persons exemption s.4(1)(b). Llama punted ("not explicitly stated") — wrong but not misleading. Neither composed the two provisions. |
| G05 | 1 | 1 | Both cited s.5(1) fines ban; both ignored the s.5(7) one-third cap half of the question. |
| G06 | 1 | 2 | Qwen quoted verbatim with no citation; Llama answered with [s.18(3)]. |
| **Grounded** | **5/12** | **6/12** | |
| C01 | 2 | 2 | Both hedged the minimum wage figure appropriately. |
| C02 | 0 | 1 | Qwen: "Labour Court" + invented ₦5M Federal High Court rule. Llama: uncertain, garbled name ("NIRC") but honest. Correct: National Industrial Court. |
| C03 | 0 | 1 | **Qwen hallmark failure**: "21 days per Section 23 of the Labour Act 2007" — invented figure, section, and year. Llama hedged honestly. Correct: 6 working days, s.18(1)(a). |
| C04 | 2 | 1 | Qwen correct (legal tender). Llama right direction but invented "s.57 cash or bank draft". |
| **Closed-book** | **4/8** | **5/8** | |
| X01 | 2 | 2 | Both perfect JSON. |
| X02 | 1 | 2 | Qwen dropped schema fields AND invented employment_years=2 (true: ~6). Llama kept full schema, honest nulls. |
| X03 | 1 | 1 | Both misrouted overtime→paye (salary distractor won). Intent router needs few-shots. |
| X04 | 2 | 0 | Qwen correctly output "unknown" for the out-of-scope tenancy query; Llama shoehorned "termination_notice". |
| **Extraction** | **6/8** | **5/8** | |
| P01 | 0 | 2 | Qwen produced gibberish ("Tu gidy tu take tu oga…") — not Pidgin, not any language. Llama: natural Pidgin, correct law, cited. |
| P02 | 1 | 2 | Qwen shaky/self-contradicting; Llama clear and correct. |
| P03 | 0 | 2 | **Qwen generation collapse**: infinite Yoruba-like token loop. Llama solid. |
| P04 | 1 | 1 | Qwen garbled but covered ~4 categories; Llama fluent but only covered (a). |
| **Pidgin** | **2/8** | **7/8** | Decisive for Pillar 3 (+15% African Alpha, localisation award). |
| F01 | 2 | 1 | Qwen exact `CLAIM … \|\| CITE` grammar; Llama split CITE onto its own line. |
| F02 | 2 | 1 | Same pattern. |
| **Format** | **4/4** | **2/4** | Qwen better grammar-compliance → GBNF will fight Llama slightly more. |
| **TOTAL** | **21/40** | **25/40** | |

## Phi-3.5-mini

| Q | Score | Notes |
|---|---|---|
| G01 | 1 | Correct (two weeks) but malformed citation path "s.11(c)" |
| G02 | 2 | **Only model to answer correctly** (pay due; absence within the 12-day cap) |
| G03 | 2 | Full (a)–(d) coverage, cited |
| G04 | 2 | **Only model to compose s.3(1) with the s.4(1)(b) exemption** |
| G05 | 2 | **Only model to apply both s.5(1) and the s.5(7) one-third cap** |
| G06 | 2 | Correct, cited |
| **Grounded** | **11/12** | Clearly the strongest statutory reasoner |
| C01 | 1 | Asserted stale wage figure with soft hedge |
| C02 | 1 | Waffle, no answer |
| C03 | 1 | Same "21 days" hallucination as Qwen, hedged |
| C04 | 1 | Waffle |
| **Closed-book** | **4/8** | |
| X01 | 1 | Correct fields but markdown fences + "Note:" prose (schema said JSON only) |
| X02 | 1 | Fences + dropped schema fields |
| X03 | 0 | `"computation": "annual"` — invalid enum value |
| X04 | 0 | Misrouted to termination_notice |
| **Extraction** | **2/8** | Worst — pervasive fence/prose wrapping |
| P01–P04 | 1,1,1,1 | Legally right, but register is generic broken English ("Yoo, yoo", "Ya need") — not Naija Pidgin |
| **Pidgin** | **4/8** | |
| F01 | 0 | Ignored CLAIM/CITE grammar entirely |
| F02 | 1 | Partial (CITE line, no CLAIM prefix, no `\|\|`) |
| **Format** | **1/4** | |
| **TOTAL** | **22/40** | |

## Final standings and speed

| Model | Score | Mean gen TPS (M1 Max) | File size |
|---|---|---|---|
| **Llama 3.2 3B Instruct** | **25/40** | 48.3 | 1.9 GB |
| Phi-3.5-mini (3.8B) | 22/40 | 38.0 | 2.2 GB |
| Qwen 2.5 3B Instruct | 21/40 | 63.9 | 1.8 GB |

## Decision: Llama 3.2 3B Instruct Q4_K_M — see docs/DECISIONS.md ADR-006

## Cross-cutting findings (feed into design)

1. **No 3B model composes two statutory provisions reliably** (G04, G05).
   Retrieval must surface exemption/proviso chunks *together* with the main
   rule (SAC cross-references), and the verifier must catch claims that cite
   a rule while ignoring its statutory exceptions.
2. **Day/date arithmetic is not model-safe** (G02, X02): the rules engine
   takes over all calendar math, including "two weeks = how many working
   days" and tenure computation from start dates.
3. **Closed-book = hallucination roulette** (C02/C03): the product must never
   answer without retrieval context. This was already the design; now it's
   evidence-backed.
4. Both models handled `-sys` + chat-template single-turn fine at temp 0.2.
