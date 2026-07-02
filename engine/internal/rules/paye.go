package rules

import (
	"fmt"
	"strings"
)

// PAYE under the Nigeria Tax Act 2025, effective 1 January 2026.
// Bands VERIFIED AGAINST THE GAZETTE Fourth Schedule text (made under
// s.58(1)) in corpus/tax/nigeria-tax-act-2025.md on 2026-07-02, matching
// PwC Worldwide Tax Summaries and KPMG Flash Alert 2025-168. Rent relief
// per NTA 2025 (20% of annual rent, capped ₦500,000). The former
// Consolidated Relief Allowance is replaced by the ₦800,000 0% band.
// Employees earning no more than the national minimum wage (₦70,000/month)
// are exempt from PAYE.

const (
	payeAsAt            = "Nigeria Tax Act 2025 (in force from 2026-01-01), rates verified against the gazette Fourth Schedule text 2026-07-02"
	minimumWageMonthly  = 70_000
	rentReliefRate      = 0.20
	rentReliefCap       = 500_000
	pensionInfoCitation = "[Pension Reform Act 2014, s.4(1)]" // 8% employee contribution, deductible
)

type payeBand struct {
	upTo float64 // cumulative upper bound of taxable income, 0 = no bound
	rate float64
}

// NTA 2025 graduated bands (annual taxable income, naira).
var payeBands = []payeBand{
	{800_000, 0.00},
	{3_000_000, 0.15},
	{12_000_000, 0.18},
	{25_000_000, 0.21},
	{50_000_000, 0.23},
	{0, 0.25},
}

// PAYEInput are the parameters the LLM extracts from the user's question.
// Only GrossAnnual is required; the rest refine the computation.
type PAYEInput struct {
	GrossAnnual   float64 `json:"gross_annual"`
	AnnualRent    float64 `json:"annual_rent,omitempty"`
	PensionAnnual float64 `json:"pension_annual,omitempty"` // employee pension contribution
}

type PAYEResult struct {
	Input         PAYEInput `json:"input"`
	Exempt        bool      `json:"exempt"`
	ExemptReason  string    `json:"exempt_reason,omitempty"`
	RentRelief    float64   `json:"rent_relief"`
	TaxableAnnual float64   `json:"taxable_annual"`
	Bands         []Line    `json:"bands"`
	AnnualTax     float64   `json:"annual_tax"`
	MonthlyTax    float64   `json:"monthly_tax"`
	EffectiveRate float64   `json:"effective_rate"`
	AsAt          string    `json:"as_at"`
}

// PAYE computes annual and monthly personal income tax.
func PAYE(in PAYEInput) PAYEResult {
	res := PAYEResult{Input: in, AsAt: payeAsAt}

	if in.GrossAnnual <= minimumWageMonthly*12 {
		res.Exempt = true
		res.ExemptReason = "gross income does not exceed the national minimum wage " +
			"(₦70,000/month), which the Nigeria Tax Act 2025 exempts from PAYE"
		return res
	}

	res.RentRelief = min(in.AnnualRent*rentReliefRate, rentReliefCap)
	res.TaxableAnnual = in.GrossAnnual - res.RentRelief - in.PensionAnnual
	if res.TaxableAnnual < 0 {
		res.TaxableAnnual = 0
	}

	remaining := res.TaxableAnnual
	prev := 0.0
	for _, b := range payeBands {
		if remaining <= 0 {
			break
		}
		width := remaining
		if b.upTo > 0 {
			width = min(remaining, b.upTo-prev)
			prev = b.upTo
		}
		tax := width * b.rate
		res.Bands = append(res.Bands, Line{
			Label:    fmt.Sprintf("₦%s at %.0f%%", formatNaira(width), b.rate*100),
			Amount:   tax,
			Citation: "[Nigeria Tax Act 2025, s.58(1) and Fourth Schedule]",
		})
		res.AnnualTax += tax
		remaining -= width
	}
	res.MonthlyTax = res.AnnualTax / 12
	if res.TaxableAnnual > 0 {
		res.EffectiveRate = res.AnnualTax / in.GrossAnnual
	}
	return res
}

// Render produces the user-facing answer deterministically (see
// NoticeResult.Render for why no LLM touches these numbers).
func (r PAYEResult) Render() string {
	var b strings.Builder
	if r.Exempt {
		fmt.Fprintf(&b, "You are **exempt from PAYE**: %s.\n\nBasis: %s.", r.ExemptReason, r.AsAt)
		return b.String()
	}
	fmt.Fprintf(&b, "Your PAYE is **₦%s per month** (₦%s per year, effective rate %.1f%%).\n\n",
		formatNaira(r.MonthlyTax), formatNaira(r.AnnualTax), r.EffectiveRate*100)
	fmt.Fprintf(&b, "Breakdown (annual):\n")
	fmt.Fprintf(&b, "- Gross income: ₦%s\n", formatNaira(r.Input.GrossAnnual))
	if r.RentRelief > 0 {
		fmt.Fprintf(&b, "- Rent relief (20%% of rent, capped ₦500,000): −₦%s [Nigeria Tax Act 2025]\n",
			formatNaira(r.RentRelief))
	}
	if r.Input.PensionAnnual > 0 {
		fmt.Fprintf(&b, "- Pension contribution: −₦%s %s\n",
			formatNaira(r.Input.PensionAnnual), pensionInfoCitation)
	}
	fmt.Fprintf(&b, "- Taxable income: ₦%s\n", formatNaira(r.TaxableAnnual))
	for _, band := range r.Bands {
		fmt.Fprintf(&b, "- %s = ₦%s %s\n", band.Label, formatNaira(band.Amount), band.Citation)
	}
	fmt.Fprintf(&b, "\nBasis: %s. Computed deterministically by Òfin's rules engine.", r.AsAt)
	return b.String()
}

func formatNaira(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
