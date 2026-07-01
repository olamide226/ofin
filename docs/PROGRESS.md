# Progress Log

## Week 1 (July 1–7, 2026) — Foundations and bake-off

Target exit criteria: model locked, hardware in hand, labour corpus in clean
text, baseline numbers recorded.

| Task | Status | Notes |
|---|---|---|
| Read DevPost rules, eligibility, IP terms | done | ⚠️ residency requirement — see action items below |
| Register on DevPost | **blocked on Ola** | needs personal account; yields `team_id` |
| Clone submission template + profiler | done | siblings of this repo in `ai_world/` |
| Secure 8 GB target hardware | **blocked on Ola** | refurb ThinkPad class, ~£120–180 |
| Model bake-off (Qwen 2.5 3B / Llama 3.2 3B / Phi-3.5-mini) | **done — Llama 3.2 3B locked** | ADR-006; scoresheet in `eval/bakeoff/SCORES.md` |
| Baseline profiler run (locked model) | done | 55.4 TPS / 902 ms / 2107 MB peak RSS on M1 Max (dev baseline, ADR-002) |
| Source + clean labour corpus | done | 5 acts in `corpus/labour/`, provenance in `sources.md` |
| Name check "Òfin" | done | no collision found (ADR-005) |

### Ola action items (July 1)

1. **Eligibility (urgent):** rules say entrants must *reside* in a listed
   African country. Confirm the citizenship/diaspora or Nigeria-HQ-company
   route before further investment. Also confirm: venture <12 months old,
   <$25k external funding, early-stage declaration.
2. **DevPost registration** → `team_id` + confirm GitHub handle for
   `metadata.json`.
3. **Order the refurb 8 GB laptop** — perf/thermal certification and the
   +10% budget-laptop claim depend on it.
