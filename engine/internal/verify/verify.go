package verify

import (
	"fmt"
	"strings"
	"sync"
)

// Verdict for one claim.
type Verdict int

const (
	Verified Verdict = iota // citation resolves, quantities supported, similarity high
	Flagged                 // weak support or unverifiable — shown with a warning
	Failed                  // citation does not resolve, or quantity contradicted
)

func (v Verdict) String() string {
	return [...]string{"verified", "flagged", "failed"}[v]
}

// Similarity thresholds for claim-vs-source embedding cosine (bge-small,
// normalized vectors). Calibrated 2026-07-02 on true/false claim pairs
// (ADR-009): true 0.70-0.82, topically-related-but-false 0.55-0.63. The
// margin is thin — this layer only reliably rejects wrong-TOPIC citations;
// numeric wrongness is the quantity layer's job and banded/interval claims
// are the rules engine's (Week 4).
const (
	simPass = 0.66
	simFlag = 0.55
)

// Result is the verifier's output for one claim.
type Result struct {
	Claim      Claim
	Verdict    Verdict
	Reasons    []string
	Similarity float64
	SourceRef  string // resolved citation label of the checked source
	SourceText string // the statutory text the claim was checked against
}

// CorpusLookup resolves a citation to section text. Implemented by the
// retrieval store.
type CorpusLookup interface {
	SectionText(actShort, sectionID string) (string, error)
}

// Embedder produces normalized embeddings (dot product = cosine).
type Embedder interface {
	Embed(text string) ([]float32, error)
}

// BatchEmbedder is optionally implemented by an Embedder (llama.Server is)
// to embed several texts in one request.
type BatchEmbedder interface {
	EmbedBatch(texts []string) ([][]float32, error)
}

// Cache memoizes embeddings keyed by exact input text. Statutory sections
// repeat both within one answer (several claims citing s.11) and across
// questions, and re-embedding them dominated verification latency on the
// 4 vCPU audit profile (143 s warm RAG+verify, 2026-07-02 VM run). Vectors
// are identical to the uncached path — zero effect on verdicts.
type Cache struct {
	mu sync.Mutex
	m  map[string][]float32
}

func NewCache() *Cache { return &Cache{m: map[string][]float32{}} }

// cacheMax bounds memory: 384-dim float32 ≈ 1.5 KB per entry → ~1.5 MB.
const cacheMax = 1024

func (c *Cache) get(text string) ([]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[text]
	return v, ok
}

func (c *Cache) put(text string, vec []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= cacheMax {
		c.m = map[string][]float32{} // reset; hot sections re-fill within one answer
	}
	c.m[text] = vec
}

// Verifier checks claims against the corpus.
type Verifier struct {
	Corpus  CorpusLookup
	Embed   Embedder
	Resolve ActResolver // maps prose act names to canonical act_short
	Cache   *Cache      // optional cross-ask embedding memo (nil = off)
	// Extra is an additional trusted source for this answer — the rules
	// engine's computation result JSON. Computed figures (₦63,500/month
	// PAYE) appear in no statute; without this they would fail the
	// quantity layer despite being deterministically correct.
	Extra string
}

// embed returns the embedding for text, consulting the cache first.
func (v *Verifier) embed(text string) ([]float32, error) {
	if v.Cache != nil {
		if vec, ok := v.Cache.get(text); ok {
			return vec, nil
		}
	}
	vec, err := v.Embed.Embed(text)
	if err == nil && v.Cache != nil {
		v.Cache.put(text, vec)
	}
	return vec, err
}

// prefetch batch-embeds every text VerifyClaim will need that isn't cached
// yet — one llama-server round-trip instead of one per claim per source.
func (v *Verifier) prefetch(claims []Claim) {
	be, ok := v.Embed.(BatchEmbedder)
	if !ok || v.Cache == nil {
		return
	}
	seen := map[string]bool{}
	var texts []string
	add := func(t string) {
		if t == "" || seen[t] {
			return
		}
		seen[t] = true
		if _, hit := v.Cache.get(t); !hit {
			texts = append(texts, t)
		}
	}
	for _, c := range claims {
		add(StripCitations(c.Text))
		for _, cit := range c.Citations {
			if text, err := v.Corpus.SectionText(cit.Act, cit.Section); err == nil {
				add(truncate(text, 1800))
			}
		}
	}
	if v.Extra != "" {
		add(truncate(v.Extra, 1800))
	}
	vecs, err := be.EmbedBatch(texts)
	if err != nil {
		return // VerifyClaim falls back to per-text embedding
	}
	for i, t := range texts {
		v.Cache.put(t, vecs[i])
	}
}

func dot(a, b []float32) float64 {
	var s float64
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

// VerifyClaim checks one claim: every citation must resolve, quantities the
// model introduced (not echoed from the question) must appear in a cited
// source, and the claim must be semantically supported by the best-matching
// cited source.
func (v *Verifier) VerifyClaim(question string, claim Claim) Result {
	res := Result{Claim: claim}

	var sources []string
	var refs []string
	for _, c := range claim.Citations {
		text, err := v.Corpus.SectionText(c.Act, c.Section)
		if err != nil {
			res.Verdict = Failed
			res.Reasons = append(res.Reasons,
				fmt.Sprintf("citation does not resolve: %s", c.Raw))
			continue
		}
		sources = append(sources, text)
		refs = append(refs, fmt.Sprintf("[%s, %s%s]", c.Act, c.Section, c.SubPath))
	}
	if len(sources) == 0 {
		res.Verdict = Failed
		return res
	}
	combined := strings.Join(sources, "\n\n")
	res.SourceRef = strings.Join(refs, " ")
	res.SourceText = combined

	assertion := StripCitations(claim.Text)

	// Deterministic layer: numeric assertions must exist in the source.
	if missing := UnsupportedQuantities(assertion, combined+"\n"+v.Extra, question); len(missing) > 0 {
		for _, q := range missing {
			unit := q.Unit
			if unit == "" {
				unit = "(no unit)"
			}
			res.Reasons = append(res.Reasons,
				fmt.Sprintf("quantity not found in cited source: %g %s", q.Value, unit))
		}
		res.Verdict = Failed
		return res
	}

	// Semantic layer: embedding similarity between claim and source.
	claimVec, err := v.embed(assertion)
	if err != nil {
		res.Verdict = Flagged
		res.Reasons = append(res.Reasons, "similarity check unavailable: "+err.Error())
		return res
	}
	if v.Extra != "" {
		sources = append(sources, v.Extra)
	}
	best := -1.0
	for _, src := range sources {
		srcVec, err := v.embed(truncate(src, 1800))
		if err != nil {
			continue
		}
		if s := dot(claimVec, srcVec); s > best {
			best = s
		}
	}
	res.Similarity = best
	switch {
	case best >= simPass:
		res.Verdict = Verified
	case best >= simFlag:
		res.Verdict = Flagged
		res.Reasons = append(res.Reasons,
			fmt.Sprintf("weak semantic support (%.2f)", best))
	default:
		res.Verdict = Failed
		res.Reasons = append(res.Reasons,
			fmt.Sprintf("claim not supported by cited text (%.2f)", best))
	}
	return res
}

// VerifyAnswer verifies every claim in an answer. Uncited sentences are
// returned so the renderer can mark them as general guidance.
func (v *Verifier) VerifyAnswer(question, answer string) ([]Result, []string) {
	claims, uncited := SegmentClaims(answer, v.Resolve)
	v.prefetch(claims)
	results := make([]Result, 0, len(claims))
	for _, c := range claims {
		results = append(results, v.VerifyClaim(question, c))
	}
	return results, uncited
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
