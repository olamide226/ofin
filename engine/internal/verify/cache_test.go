package verify

import (
	"sync/atomic"
	"testing"
)

// countingEmbedder tracks how many texts were actually embedded, via either
// path. Fixed vector keeps the semantic layer deterministic (sim = 1.0).
type countingEmbedder struct {
	single  atomic.Int64 // Embed calls
	batched atomic.Int64 // texts embedded through EmbedBatch
}

func (c *countingEmbedder) Embed(string) ([]float32, error) {
	c.single.Add(1)
	return []float32{1, 0, 0}, nil
}

func (c *countingEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	c.batched.Add(int64(len(texts)))
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{1, 0, 0}
	}
	return out, nil
}

// Three claims citing the same section: the source must be embedded once,
// not three times, and everything must go through the batch path.
func TestVerifyAnswerCachesSourceEmbeddings(t *testing.T) {
	emb := &countingEmbedder{}
	v := &Verifier{
		Corpus: fakeCorpus{"Labour Act 2004|s.11": s11},
		Embed:  emb,
		Cache:  NewCache(),
	}
	answer := "Notice may end the contract [Labour Act 2004, s.11]. " +
		"Payment in lieu of notice is allowed [Labour Act 2004, s.11]. " +
		"Waiving the right to notice is permitted [Labour Act 2004, s.11]."

	results, _ := v.VerifyAnswer("what notice applies?", answer)
	if len(results) != 3 {
		t.Fatalf("expected 3 claims, got %d", len(results))
	}
	for _, r := range results {
		if r.Verdict != Verified {
			t.Errorf("claim %q verdict = %v, want Verified", r.Claim.Text, r.Verdict)
		}
	}
	// 3 unique claim texts + 1 unique source = 4 embeds, all via prefetch.
	if got := emb.batched.Load(); got != 4 {
		t.Errorf("batched embeds = %d, want 4 (3 claims + 1 shared source)", got)
	}
	if got := emb.single.Load(); got != 0 {
		t.Errorf("single embeds = %d, want 0 (everything prefetched)", got)
	}

	// Second answer citing the same section: source vector comes from cache.
	emb.batched.Store(0)
	v.VerifyAnswer("again?", "A contract needs notice to terminate [Labour Act 2004, s.11].")
	if got := emb.batched.Load(); got != 1 {
		t.Errorf("batched embeds on 2nd answer = %d, want 1 (claim only; source cached)", got)
	}
}

// Without a Cache the verifier must behave exactly as before (per-text
// embedding, no prefetch) — the CLI keeps working if wiring is missed.
func TestVerifyClaimWorksWithoutCache(t *testing.T) {
	emb := &countingEmbedder{}
	v := &Verifier{
		Corpus: fakeCorpus{"Labour Act 2004|s.11": s11},
		Embed:  emb,
	}
	results, _ := v.VerifyAnswer("q?", "Notice may end the contract [Labour Act 2004, s.11].")
	if len(results) != 1 || results[0].Verdict != Verified {
		t.Fatalf("unexpected results: %+v", results)
	}
	if got := emb.batched.Load(); got != 0 {
		t.Errorf("batched embeds = %d, want 0 without cache", got)
	}
	if got := emb.single.Load(); got != 2 {
		t.Errorf("single embeds = %d, want 2 (claim + source)", got)
	}
}
