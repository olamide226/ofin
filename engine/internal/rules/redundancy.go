package rules

import "fmt"

// RedundancyResult is the statutory entitlement breakdown for a redundancy
// under Labour Act 2004 s.20.
//
// DELIBERATE DESIGN NOTE: the Labour Act prescribes NO severance formula.
// s.20(1)(c) creates a duty to NEGOTIATE redundancy payments, and s.20(2)
// lets the Minister regulate amounts (no such regulation is in the corpus).
// A calculator that output a severance amount would be inventing law — the
// exact failure Òfin exists to prevent. What IS deterministic: the s.20
// process rights, the s.20(3) definition, and the s.11 notice band computed
// from tenure. The master plan's "severance calculator" was corrected to
// this entitlement breakdown after reading the gazette text (2026-07-04).
type RedundancyResult struct {
	TenureMonths float64       `json:"tenure_months,omitempty"`
	Definition   string        `json:"definition"`
	Rights       []Line        `json:"rights"`
	Notice       *NoticeResult `json:"notice,omitempty"` // s.11 band when tenure is known
	AsAt         string        `json:"as_at"`
}

// Redundancy builds the entitlement breakdown. tenureMonths <= 0 means the
// tenure is unknown: the process rights still render, the notice band is
// omitted.
func Redundancy(tenureMonths float64) RedundancyResult {
	res := RedundancyResult{
		TenureMonths: tenureMonths,
		Definition:   `"redundancy" means an involuntary and permanent loss of employment caused by an excess of manpower`,
		Rights: []Line{
			{Label: "The employer must inform your trade union or workers' representative of the reasons for and extent of the redundancy",
				Citation: "[Labour Act 2004, s.20(1)(a)]"},
			{Label: `Discharge must follow "last in, first out" within the affected category, subject to relative merit (skill, ability, reliability)`,
				Citation: "[Labour Act 2004, s.20(1)(b)]"},
			{Label: "The employer must use their best endeavours to NEGOTIATE redundancy payments — the Act sets a duty to negotiate, not a fixed amount",
				Citation: "[Labour Act 2004, s.20(1)(c)]"},
		},
		AsAt: "Labour Act, Cap L1 LFN 2004 (as at July 2026)",
	}
	if tenureMonths > 0 {
		n := NoticePeriod(tenureMonths)
		res.Notice = &n
	}
	return res
}

func (r RedundancyResult) Summary() string {
	s := "redundancy entitlements: union notice, last-in-first-out, negotiated payments [Labour Act 2004, s.20]"
	if r.Notice != nil {
		s += fmt.Sprintf("; plus %s notice %s", r.Notice.Notice, r.Notice.Citation)
	}
	return s
}

func (r RedundancyResult) Render() string {
	var b []byte
	b = fmt.Appendf(b, "In a redundancy (%s [Labour Act 2004, s.20(3)]), your statutory entitlements are:\n\n",
		r.Definition)
	for _, line := range r.Rights {
		b = fmt.Appendf(b, "- %s %s\n", line.Label, line.Citation)
	}
	b = fmt.Appendf(b, "\nNote: the Labour Act does NOT fix a severance amount — redundancy pay comes from "+
		"negotiation, your contract, or any collective agreement [Labour Act 2004, s.20(1)(c), s.20(2)].\n")
	if r.Notice != nil {
		b = fmt.Appendf(b, "\nSeparately, with %.1f years of service you are still entitled to **%s** notice "+
			"of termination (or payment in lieu) %s.\n", r.Notice.TenureMonths/12, r.Notice.Notice, r.Notice.Citation)
	}
	b = fmt.Appendf(b, "\nBasis: %s. Computed deterministically by Òfin's rules engine.", r.AsAt)
	return string(b)
}
