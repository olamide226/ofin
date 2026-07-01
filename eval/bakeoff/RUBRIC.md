# Bake-off Scoring Rubric

20 questions × 3 models, each answer scored 0–2. Max 40 per model.
Manual scoring against the `expected` field in `questions.jsonl`.

## Per-category criteria

| Category (n) | 2 points | 1 point | 0 points |
|---|---|---|---|
| **grounded** (6) | Legally correct per the provided text AND every claim carries a well-formed citation | Correct but citations missing/malformed, or minor over-reach beyond the text | Wrong, hallucinated beyond the text, or ignores the provided statute |
| **closed_book** (4) | Correct; or wrong-but-explicitly-hedged where the rubric allows | Wrong with hedging / partially right | Confidently wrong (the cardinal sin for this product) |
| **extraction** (4) | Valid bare JSON, all fields correct | Valid JSON, one minor field error | Invalid JSON, prose wrapper, or shoehorned wrong computation |
| **pidgin** (4) | Legally correct + cited + natural Pidgin | Correct but stilted/anglicized Pidgin, or Pidgin fine but citation lost | Wrong law, or answer not actually in Pidgin |
| **format** (2) | Every line matches the CLAIM/CITE grammar exactly | Right content, small format drift | Prose or markdown; grammar ignored |

## Weighting note

Categories are deliberately sized by production importance: grounded QA (6)
and extraction (4) carry the product; closed-book (4) mostly measures
hallucination *confidence* (we always retrieve at runtime); format (2)
predicts whether GBNF grammar enforcement (Week 3) will fight the model.

## Tie-breakers (in order)

1. Grounded + format subtotal (citation discipline is the product)
2. Extraction subtotal (rules-engine integration)
3. Pidgin quality
4. Generation TPS from results.csv
5. License friendliness (Apache-2.0 Qwen > MIT Phi > Llama Community)
