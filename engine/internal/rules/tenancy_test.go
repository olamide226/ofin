package rules

import (
	"strings"
	"testing"
)

// Bands hand-checked against the gazette text of s.13(1), Lagos Tenancy
// Law 2011 (corpus/tenancy/tenancy-law-lagos-2011.md).
func TestTenancyNoticeBands(t *testing.T) {
	cases := []struct {
		ty     TenancyType
		notice string
		cite   string
	}{
		{TenancyAtWill, "a week's notice", "s.13(1)(a)"},
		{TenancyMonthly, "one month's notice", "s.13(1)(b)"},
		{TenancyQuarterly, "three months' notice", "s.13(1)(c)"},
		{TenancyHalfYearly, "three months' notice", "s.13(1)(d)"},
		{TenancyYearly, "six months' notice", "s.13(1)(e)"},
	}
	for _, c := range cases {
		res, ok := TenancyNotice(c.ty)
		if !ok {
			t.Fatalf("%s: expected ok", c.ty)
		}
		if res.Notice != c.notice || !strings.Contains(res.Citation, c.cite) {
			t.Errorf("%s: got %q %q, want %q citing %s", c.ty, res.Notice, res.Citation, c.notice, c.cite)
		}
		if !strings.Contains(res.Render(), "LAGOS STATE") {
			t.Errorf("%s: rendered answer must flag Lagos jurisdiction", c.ty)
		}
		if !strings.Contains(res.Render(), "DEFAULT") {
			t.Errorf("%s: rendered answer must carry the no-stipulation caveat", c.ty)
		}
	}
}

func TestTenancyFixedTermNeedsNoQuitNotice(t *testing.T) {
	res, ok := TenancyNotice(TenancyFixedTerm)
	if !ok {
		t.Fatal("expected ok")
	}
	r := res.Render()
	if !strings.Contains(r, "no notice to quit") || !strings.Contains(r, "7-day") {
		t.Errorf("fixed term must render the s.13(5) rule (no quit notice + 7-day intention), got: %s", r)
	}
}

func TestTenancyArrearsNotes(t *testing.T) {
	monthly, _ := TenancyNotice(TenancyMonthly)
	if !strings.Contains(monthly.Render(), "six months in arrears") {
		t.Error("monthly must carry the s.13(2) six-month arrears lapse note")
	}
	quarterly, _ := TenancyNotice(TenancyQuarterly)
	if !strings.Contains(quarterly.Render(), "one year in arrears") {
		t.Error("quarterly must carry the s.13(3) one-year arrears lapse note")
	}
}

func TestTenancyUnknownType(t *testing.T) {
	if _, ok := TenancyNotice("weekly"); ok {
		t.Error("unknown tenancy type must not compute")
	}
}

// s.20 prescribes NO severance amount — the result must say so, never
// invent a figure, and still fold in the tenure-based s.11 notice band.
func TestRedundancyEntitlements(t *testing.T) {
	res := Redundancy(72) // 6 years
	r := res.Render()
	for _, want := range []string{
		"last in, first out", "s.20(1)(b)",
		"NEGOTIATE", "s.20(1)(c)",
		"does NOT fix a severance amount",
		"one month", "s.11(2)(d)", // 6 years -> one month notice
	} {
		if !strings.Contains(r, want) {
			t.Errorf("redundancy render missing %q", want)
		}
	}
	if strings.ContainsAny(r, "₦") {
		t.Error("redundancy must never render a naira amount — the Act prescribes none")
	}
}

func TestRedundancyWithoutTenure(t *testing.T) {
	res := Redundancy(0)
	if res.Notice != nil {
		t.Error("unknown tenure must omit the notice band, not guess one")
	}
	if !strings.Contains(res.Render(), "s.20(1)(a)") {
		t.Error("process rights must render even without tenure")
	}
}
