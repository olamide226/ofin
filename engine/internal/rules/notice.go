// Package rules is Òfin's deterministic computation engine (Pillar 2).
// Statutory arithmetic — notice periods, PAYE bands, severance — is pure
// code encoding the statutes; the LLM extracts parameters and narrates
// results, never calculates. This exists because graduated-band claims are
// provably invisible to the citation verifier (ADR-009) and 3B models
// cannot map tenure to bands reliably (bake-off G01, Week-2 failures).
package rules

import "fmt"

// Line is one row of a computation breakdown, with its statutory basis.
type Line struct {
	Label    string  `json:"label"`
	Amount   float64 `json:"amount,omitempty"`
	Citation string  `json:"citation"`
}

// NoticeResult is the statutory minimum notice for terminating a contract
// of employment under the Labour Act.
type NoticeResult struct {
	TenureMonths float64 `json:"tenure_months"`
	Notice       string  `json:"notice"`
	Citation     string  `json:"citation"`
	Notes        []Line  `json:"notes"`
	AsAt         string  `json:"as_at"`
}

// NoticePeriod maps continuous-employment tenure to the s.11(2) bands.
// Band boundaries follow the statutory text exactly: "three months or
// less" / "more than three months but less than two years" / "two years
// but less than five years" / "five years or more".
func NoticePeriod(tenureMonths float64) NoticeResult {
	var notice, cite string
	switch {
	case tenureMonths <= 3:
		notice, cite = "one day", "[Labour Act 2004, s.11(2)(a)]"
	case tenureMonths < 24:
		notice, cite = "one week", "[Labour Act 2004, s.11(2)(b)]"
	case tenureMonths < 60:
		notice, cite = "two weeks", "[Labour Act 2004, s.11(2)(c)]"
	default:
		notice, cite = "one month", "[Labour Act 2004, s.11(2)(d)]"
	}
	notes := []Line{
		{Label: "Notice of one week or more must be given in writing",
			Citation: "[Labour Act 2004, s.11(3)]"},
		{Label: "Either party may accept payment in lieu of notice",
			Citation: "[Labour Act 2004, s.11(6)]"},
		{Label: "Payment in lieu is calculated on money wages only, excluding overtime and other allowances",
			Citation: "[Labour Act 2004, s.11(9)]"},
	}
	return NoticeResult{
		TenureMonths: tenureMonths,
		Notice:       notice,
		Citation:     cite,
		Notes:        notes,
		AsAt:         "Labour Act, Cap L1 LFN 2004 (as at July 2026)",
	}
}

func (r NoticeResult) Summary() string {
	return fmt.Sprintf("statutory minimum notice: %s %s", r.Notice, r.Citation)
}

// Render produces the user-facing answer deterministically. No LLM touches
// these numbers: a 3B model recomputes figures it was told to transcribe
// (observed live), so the numeric core of a computation answer is rendered
// by code and only optionally wrapped in model prose.
func (r NoticeResult) Render() string {
	var b []byte
	b = fmt.Appendf(b, "With %.1f years of continuous employment, the statutory minimum notice "+
		"your employer must give you (or pay in lieu of) is **%s** %s.\n\n",
		r.TenureMonths/12, r.Notice, r.Citation)
	for _, n := range r.Notes {
		b = fmt.Appendf(b, "- %s %s\n", n.Label, n.Citation)
	}
	b = fmt.Appendf(b, "\nBasis: %s. Computed deterministically by Òfin's rules engine.", r.AsAt)
	return string(b)
}
