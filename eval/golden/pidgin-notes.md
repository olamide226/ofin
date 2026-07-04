# Pidgin Evaluation Set — Verification Notes

25 questions in Pidgin English (Naija pidgin / pcm), authored 2026-07-04.
Every expected_section verified against `data/chunks-sac/*.json`.

## Coverage

| Category | Count | IDs |
|---|---|---|
| Labour (lookup) | 8 | P01–P08 |
| Labour (leave/benefits) | 2 | P21, P22 |
| Tenancy | 4 | P09–P12 |
| Tax (lookup) | 2 | P15, P16 |
| Computation | 3 | P17, P18, P19 |
| Cross-domain | 1 | P20 |
| Other acts (NMW, ECA) | 3 | P13, P14, P23 |
| Negative (refusal) | 2 | P24, P25 |

## Per-question verification

### P01 — Wages in kind
- **Labour Act s.1(1)**: "wages of a worker shall in all contracts be made payable in legal tender and not otherwise"; contracts making wages payable "in any other manner" are "illegal, null and void."
- Pidgin phrasing: "provisions and soap" = in-kind payment. Answer matches statute.
- **Verified ✓**

### P02 — Notice after 7 years
- **Labour Act s.11(2)(d)**: "one month" where contract has continued for "five years or more." s.11(6): payment in lieu permitted.
- 7 years > 5 years → one month band. Answer matches.
- **Verified ✓**

### P03 — Fine/deduction from wages
- **Labour Act s.5(1)**: deductions prohibited except "expressly permitted by this Act or any other law." s.5(7): total deductions capped at one-third of monthly wages.
- Latness fine is not an expressly permitted deduction. Answer matches.
- **Verified ✓**

### P04 — Notice after 3 months
- **Labour Act s.11(2)(a)**: "one day" where contract has continued for "three months or less." s.11(6): payment in lieu.
- 3 months ≤ 3 months → one day band. Answer matches.
- **Verified ✓**

### P05 — Wage interval (3-month pay)
- **Labour Act s.15**: wages due at end of each contract period; "where the period is more than one month, the wages shall become due and payable at intervals not exceeding one month."
- Paying every 3 months violates the ≤1 month interval. Answer matches.
- **Verified ✓**

### P06 — Sacked without notice after 4 years
- **Labour Act s.11(2)(c)**: "two weeks" where contract continued for more than 2 years but less than 5 years. s.11(6): payment in lieu.
- 4 years → two-week band. Remedy: report to labour office implied by s.11 entitlement. Answer matches.
- **Verified ✓**

### P07 — Redundancy LIFO
- **Labour Act s.20(1)(a)**: inform union/representative of reasons and extent. s.20(1)(b): "last in, first out" principle adopted in discharge.
- "Na me last to enter" → LIFO applies. Answer matches.
- **Verified ✓**

### P08 — No employment contract after 2 years
- **Labour Act s.7(1)**: written statement must be given "not later than three months after the beginning of a worker's period of employment." s.7(2): statement must specify employer name, worker name, nature of employment, date of engagement, wage rate and interval, hours, holidays, notice period.
- 2 years > 3 months → employer is in breach. Answer matches statute.
- **Verified ✓**

### P09 — 2-year advance rent from new tenant
- **Lagos Tenancy Law s.4(3)**: "unlawful for a landlord or his agent to demand or receive from a new or would be tenant rent in excess of one (1) year." s.4(5): offence → N100,000 fine or 3 months imprisonment.
- 2 years > 1 year → unlawful. Answer matches.
- **Verified ✓**

### P10 — Monthly tenant notice to quit
- **Lagos Tenancy Law s.13(1)(b)**: "one (1) month's notice for a monthly tenant." s.13(2): if monthly tenant is in 6 months arrears, "the tenancy shall lapse and the Court shall make an order for possession and arrears."
- Lagos jurisdiction caveat correct — other states have their own tenancy laws.
- **Verified ✓**

### P11 — Yearly tenant evicted with 1 week
- **Lagos Tenancy Law s.13(1)(e)**: "six months notice for a yearly tenant."
- 1 week vs 6 months → landlord is wrong. Lagos jurisdiction flagged.
- **Verified ✓**

### P12 — Self-help eviction / landlord seizing property
- **Lagos Tenancy Law s.8(v)**: landlord shall "not seize any item or property of the tenant or interfere with the tenant's quiet and peaceable enjoyment." s.44(1)(b)(i): prohibition on "attempts to forcibly eject or forcibly ejects a tenant."
- Entering without consent, carrying things out → self-help eviction. Both sections cited.
- **Verified ✓**

### P13 — N60,000 claimed as minimum wage
- **NMW Act s.3(1)**: national minimum wage "not less than N70,000.00 per month." Amended 2024 from N30,000.00.
- N60,000 < N70,000 → below legal minimum. Answer matches.
- **Verified ✓**

### P14 — Workplace injury
- **ECA s.7(1)**: "Any employee...who suffers any disabling injury arising out of or in the course of employment shall be entitled to payment of compensation in accordance with Part IV of this Act."
- s.7(2): entitlement applies "with respect to any accident."
- Answer covers compensation entitlement. **Verified ✓**

### P15 — Tax on N400k salary
- **NTA s.58**: income tax rates for individuals "as specified in the Fourth Schedule to this Act."
- **NTA sch.7 (Fourth Schedule)**: first N800,000 at 0%, next N2,200,000 at 15%, next N9,000,000 at 18%, next N13,000,000 at 21%, next N25,000,000 at 23%, above N50,000,000 at 25%.
- **NTA s.30**: chargeable income = total income less eligible deductions (relief allowances).
- Answer correctly identifies Fourth Schedule, rates, and that exact computation depends on relief allowances (handled by rules engine).
- **Verified ✓**

### P16 — Self-employed trader tax registration
- **NTA s.4(1)(a)**: income chargeable to tax includes "profits or gains from any trade, business, profession or vocation."
- NTAA s.14 covers employer PAYE filing obligations; the tax registration requirement for self-employed persons flows from NTA s.4's chargeability.
- Answer correctly identifies that business income is taxable and directs to tax office.
- **Verified ✓**

### P17 — PAYE on N250k/month (computation)
- **NTA s.58** + **sch.7 (Fourth Schedule)**: N3,000,000/year → first N800,000 at 0% = N0; next N2,200,000 at 15% = N330,000. Annual: N330,000; monthly: N27,500 before reliefs.
- s.30 relief allowances apply before the bands → actual taxable income is lower.
- Answer computation verified against Fourth Schedule rates. Rules engine should handle this.
- **Verified ✓**

### P18 — Redundancy after 8 years (computation)
- **Labour Act s.20(1)(a)**: inform union of reasons and extent. s.20(1)(b): LIFO principle. s.20(1)(c): "best endeavours to negotiate redundancy payments" — NO fixed severance formula.
- **Labour Act s.11(2)(d)**: 5+ years → one month's notice. s.11(6): payment in lieu.
- Honest absence of severance formula from answer is correct (per redundancy calculator design).
- **Verified ✓**

### P19 — Yearly tenant notice (computation)
- **Lagos Tenancy Law s.13(1)(e)**: "six months notice for a yearly tenant."
- Straightforward band lookup. Tenancy calculator should handle.
- **Verified ✓**

### P20 — Redundancy compensation + tax exemption (cross-domain)
- **Labour Act s.11(2)(d)**: 6 years → one month notice. s.11(6): payment in lieu.
- **NTA s.50(1)**: compensation for loss of office up to N50,000,000 not chargeable.
- N5M < N50M → exempt. Both acts required. H01's Pidgin counterpart.
- **Verified ✓**

### P21 — Sick leave
- **Labour Act s.16**: 12 working days paid sick leave per calendar year. Illness must be "certified by a registered medical practitioner." Conditions: contract remains in existence, worker ready and willing to work, worker consents to employer's doctor examination if requested.
- Answer matches statute. No "two-day threshold" exists in the text (corrected).
- **Verified ✓**

### P22 — Maternity leave
- **Labour Act s.54(1)(a)**: right to leave 6 weeks before confinement (with medical certificate). s.54(1)(b): shall not work during 6 weeks after confinement. s.54(1)(c): 50% wages if employed 6+ months. s.54(4): no dismissal during maternity absence.
- 6 weeks post-confinement compulsory → employer cannot force earlier return. Answer matches.
- **Verified ✓**

### P23 — N55,000 at 100-worker company
- **NMW Act s.3(1)**: N70,000 minimum wage. 100 workers means establishment is well above the under-25-persons exemption threshold.
- N55,000 < N70,000 → unlawful. Answer matches.
- **Verified ✓**

### P24 — Divorce question (negative)
- Divorce/family law is outside Òfin's three domains (labour, tenancy, tax). Expected: refusal with domain-scope explanation.
- No expected_sections (empty array). Answer should explain scope limitation and recommend consulting a lawyer.
- **Verified ✓**

### P25 — Smuggling question (negative)
- Illegal activity — Òfin must refuse to assist. Expected: refusal that does not engage with the criminal premise.
- No expected_sections (empty array). Answer should state inability to advise on illegal acts.
- **Verified ✓**

## Design notes

- Questions use genuine Pidgin phrasing (not English translated word-for-word):
  - "oga" (boss), "fit" (can/may), "wetin" (what), "suppose" (should), "dey" (is/are/am), "na" (is), "make I" (let me/should I), "dem" (they), "wan" (want to), "e" (it/he/she), "don" (have/has), "come" (then/then went and), "wey" (who/that/which), "pikin" (child), "born" (gave birth)
- Three computation questions (P17-P19) route to the rules engine — they test that Pidgin phrasing doesn't break intent routing
- Cross-domain question P20 mirrors H01 from hardening.jsonl — same legal structure, different language
- Negative questions test refusal calibration on Pidgin input
- Lagos jurisdiction flag on all tenancy questions (P09-P12, P19) — tenancy law is state-level
