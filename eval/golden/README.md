# Golden Evaluation Set

The anti-overfitting insurance: if the system is genuinely general across
this set, the organisers' hidden prompts hold no fear.

`labour.jsonl` — 40 questions (33 answerable + 3 negative controls + 4
sub-variants in Pidgin): 20 Labour Act, 6 NMW, 8 ECA, 3 TDA, 3 negatives.
Every `expected_sections` entry was verified against the chunked corpus
(`data/chunks/*.json` section-title map), not from memory.

Fields per line:
- `question` / `language` (`en` | `pcm`)
- `category` — `lookup` (single section), `cross-section` (compose ≥2
  provisions — the hard class), `remedy` ("what can I do"-shaped),
  `negative` (correct behaviour is refusal)
- `expected_sections` — the citation targets; retrieval recall and citation
  precision are measured against these
- `expected_answer` — scoring notes for the answer-accuracy pass

## Metrics (Week 3 harness, `eval/run_golden.py` — to be built)

1. **Retrieval recall@6** — expected section present in the fused top-6.
2. **Citation precision** — of citations shown, % that exist AND support
   the claim (verifier-assisted once Week 3 lands). Target: >90%.
3. **Answer accuracy** — manual 0-2 vs expected_answer.
4. **Refusal calibration** — negatives refused, answerables not refused
   (Week-2 lesson: rule-3 phrasing made the model refuse "what can I do"
   questions; both failure directions matter).

Tenancy and tax sets (30 questions incl. 10 cross-domain + 10 computation)
join in Week 4 to reach the plan's 90.
