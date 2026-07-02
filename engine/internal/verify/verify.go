package verify

import (
	"fmt"
	"strings"
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

// Verifier checks claims against the corpus.
type Verifier struct {
	Corpus  CorpusLookup
	Embed   Embedder
	Resolve ActResolver // maps prose act names to canonical act_short
	// Extra is an additional trusted source for this answer — the rules
	// engine's computation result JSON. Computed figures (₦63,500/month
	// PAYE) appear in no statute; without this they would fail the
	// quantity layer despite being deterministically correct.
	Extra string
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
	claimVec, err := v.Embed.Embed(assertion)
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
		srcVec, err := v.Embed.Embed(truncate(src, 1800))
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
