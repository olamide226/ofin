package rules

import (
	"math"
	"testing"
)

// Band boundaries follow s.11(2) text exactly. The 4-years case is the
// captured Week-2 failure the whole package exists to prevent.
func TestNoticeBands(t *testing.T) {
	cases := []struct {
		months float64
		want   string
	}{
		{1, "one day"},
		{3, "one day"},    // "three months or less" — inclusive
		{3.1, "one week"}, // "more than three months"
		{23, "one week"},  // "less than two years"
		{24, "two weeks"}, // "two years but less than five years"
		{36, "two weeks"}, // the bake-off G01 case
		{48, "two weeks"}, // the Week-2 CLI failure case (model said one month)
		{59, "two weeks"},
		{60, "one month"}, // "five years or more"
		{120, "one month"},
	}
	for _, c := range cases {
		if got := NoticePeriod(c.months); got.Notice != c.want {
			t.Errorf("NoticePeriod(%v months) = %q, want %q", c.months, got.Notice, c.want)
		}
	}
}

func approx(a, b float64) bool { return math.Abs(a-b) < 0.51 }

// Hand-computed against the NTA 2025 bands (0% to 800k, 15% next 2.2M,
// 18% next 9M, 21% next 13M, 23% next 25M, 25% above 50M).
func TestPAYE(t *testing.T) {
	cases := []struct {
		name       string
		in         PAYEInput
		wantAnnual float64
		exempt     bool
	}{
		{"minimum wage earner exempt", PAYEInput{GrossAnnual: 840_000}, 0, true},
		{"just above exemption", PAYEInput{GrossAnnual: 900_000}, 15_000, false}, // 100k over 800k @15%
		{"450k/month (bake-off X01)", PAYEInput{GrossAnnual: 5_400_000},
			330_000 + 0.18*2_400_000, false}, // 762,000
		{"450k/month with 800k rent", PAYEInput{GrossAnnual: 5_400_000, AnnualRent: 800_000},
			330_000 + 0.18*(2_400_000-160_000), false}, // relief 160k -> 733,200
		{"rent relief caps at 500k", PAYEInput{GrossAnnual: 10_000_000, AnnualRent: 5_000_000},
			330_000 + 0.18*(10_000_000-500_000-3_000_000), false}, // relief min(1M, 500k)
		{"20M crosses 21% band", PAYEInput{GrossAnnual: 20_000_000},
			330_000 + 0.18*9_000_000 + 0.21*(20_000_000-12_000_000), false}, // 3,630,000
		{"60M reaches top rate", PAYEInput{GrossAnnual: 60_000_000},
			330_000 + 1_620_000 + 2_730_000 + 5_750_000 + 0.25*10_000_000, false},
	}
	for _, c := range cases {
		got := PAYE(c.in)
		if got.Exempt != c.exempt {
			t.Errorf("%s: exempt = %v, want %v", c.name, got.Exempt, c.exempt)
			continue
		}
		if !approx(got.AnnualTax, c.wantAnnual) {
			t.Errorf("%s: annual tax = %.2f, want %.2f", c.name, got.AnnualTax, c.wantAnnual)
		}
	}
}

func TestPAYEMonthlyAndBreakdown(t *testing.T) {
	got := PAYE(PAYEInput{GrossAnnual: 5_400_000})
	if !approx(got.MonthlyTax, 63_500) {
		t.Errorf("monthly = %.2f, want 63500", got.MonthlyTax)
	}
	if len(got.Bands) != 3 {
		t.Errorf("expected 3 band lines (0%%, 15%%, 18%%), got %d: %+v", len(got.Bands), got.Bands)
	}
	var sum float64
	for _, b := range got.Bands {
		sum += b.Amount
	}
	if !approx(sum, got.AnnualTax) {
		t.Errorf("band lines sum %.2f != annual tax %.2f", sum, got.AnnualTax)
	}
}
