package rules

import "fmt"

// TenancyType is the statutory tenancy classification of s.13, Lagos
// Tenancy Law 2011. Per s.13(6), the nature of a tenancy is determined by
// reference to when rent is paid or demanded, absent contrary evidence.
type TenancyType string

const (
	TenancyAtWill     TenancyType = "at_will"
	TenancyMonthly    TenancyType = "monthly"
	TenancyQuarterly  TenancyType = "quarterly"
	TenancyHalfYearly TenancyType = "half_yearly"
	TenancyYearly     TenancyType = "yearly"
	TenancyFixedTerm  TenancyType = "fixed_term"
)

// TenancyNoticeResult is the statutory notice to determine a tenancy under
// the Lagos Tenancy Law 2011 (Lagos State only — jurisdiction is part of
// the answer, per the honesty-as-differentiator corpus policy).
type TenancyNoticeResult struct {
	TenancyType TenancyType `json:"tenancy_type"`
	Notice      string      `json:"notice"`
	Citation    string      `json:"citation"`
	Notes       []Line      `json:"notes"`
	AsAt        string      `json:"as_at"`
}

// TenancyNotice maps a tenancy type to the s.13(1) default notice bands.
// The bands apply only "where there is no stipulation as to the notice to
// be given by either party" — the contract overrides; that caveat is part
// of the rendered answer. Fixed-term tenancies need no notice to quit at
// all: s.13(5) requires instead a 7-day written notice of intention to
// recover possession (Form TL5) once the term expires.
func TenancyNotice(t TenancyType) (TenancyNoticeResult, bool) {
	res := TenancyNoticeResult{
		TenancyType: t,
		AsAt:        "Tenancy Law of Lagos State, Law No. 14 of 2011 (as at July 2026)",
	}
	switch t {
	case TenancyAtWill:
		res.Notice, res.Citation = "a week's notice", "[Lagos Tenancy Law 2011, s.13(1)(a)]"
	case TenancyMonthly:
		res.Notice, res.Citation = "one month's notice", "[Lagos Tenancy Law 2011, s.13(1)(b)]"
		res.Notes = append(res.Notes, Line{
			Label:    "If the tenant is six months in arrears of rent, the tenancy lapses and the Court shall order possession and arrears on proof",
			Citation: "[Lagos Tenancy Law 2011, s.13(2)]"})
	case TenancyQuarterly:
		res.Notice, res.Citation = "three months' notice", "[Lagos Tenancy Law 2011, s.13(1)(c)]"
		res.Notes = append(res.Notes, arrearsOneYearNote, anniversaryNote)
	case TenancyHalfYearly:
		res.Notice, res.Citation = "three months' notice", "[Lagos Tenancy Law 2011, s.13(1)(d)]"
		res.Notes = append(res.Notes, arrearsOneYearNote, anniversaryNote)
	case TenancyYearly:
		res.Notice, res.Citation = "six months' notice", "[Lagos Tenancy Law 2011, s.13(1)(e)]"
		res.Notes = append(res.Notes, anniversaryNote)
	case TenancyFixedTerm:
		res.Notice = "no notice to quit"
		res.Citation = "[Lagos Tenancy Law 2011, s.13(5)]"
		res.Notes = append(res.Notes, Line{
			Label:    "Once the term expires by effluxion of time, the landlord must instead serve a 7-day written notice of intention to apply to recover possession (Form TL5)",
			Citation: "[Lagos Tenancy Law 2011, s.13(5)]"})
	default:
		return res, false
	}
	res.Notes = append(res.Notes,
		Line{Label: "These are the DEFAULT periods — they apply only where the tenancy agreement does not stipulate its own notice",
			Citation: "[Lagos Tenancy Law 2011, s.13(1)]"},
		Line{Label: "The nature of a tenancy is determined by when rent is paid or demanded, absent contrary evidence",
			Citation: "[Lagos Tenancy Law 2011, s.13(6)]"})
	return res, true
}

var arrearsOneYearNote = Line{
	Label:    "If the tenant is one year in arrears of rent, the tenancy lapses and the Court shall order possession and arrears on proof",
	Citation: "[Lagos Tenancy Law 2011, s.13(3)]",
}

var anniversaryNote = Line{
	Label:    "The notice need not terminate on the anniversary of the tenancy; it may terminate on or after the expiration date",
	Citation: "[Lagos Tenancy Law 2011, s.13(4)]",
}

var tenancyLabels = map[TenancyType]string{
	TenancyAtWill: "tenant at will", TenancyMonthly: "monthly tenant",
	TenancyQuarterly: "quarterly tenant", TenancyHalfYearly: "half-yearly tenant",
	TenancyYearly: "yearly tenant", TenancyFixedTerm: "fixed-term tenancy",
}

func (r TenancyNoticeResult) Summary() string {
	return fmt.Sprintf("statutory default notice for a %s: %s %s",
		tenancyLabels[r.TenancyType], r.Notice, r.Citation)
}

func (r TenancyNoticeResult) Render() string {
	var b []byte
	b = fmt.Appendf(b, "Under LAGOS STATE law, the default notice to end a tenancy for a %s is **%s** %s.\n\n",
		tenancyLabels[r.TenancyType], r.Notice, r.Citation)
	for _, n := range r.Notes {
		b = fmt.Appendf(b, "- %s %s\n", n.Label, n.Citation)
	}
	b = fmt.Appendf(b, "\nJurisdiction: Lagos State only — other states have their own tenancy laws.\n"+
		"Basis: %s. Computed deterministically by Òfin's rules engine.", r.AsAt)
	return string(b)
}
