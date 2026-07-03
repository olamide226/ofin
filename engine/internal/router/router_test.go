package router

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)

func f(v float64) *float64 { return &v }
func s(v string) *string   { return &v }

// Captured failure: extractor invented employment_years=3 for "since March
// 2020"; the start date must win (6.3 years -> one month's notice).
func TestStartDateOutranksModelYears(t *testing.T) {
	p := &Params{Computation: "termination_notice",
		EmploymentYears: f(3), EmploymentStart: s("March 2020")}
	out, ok := Computation(p, "how much notice and tax do I get if they sack me?", now)
	if !ok {
		t.Fatal("expected computation")
	}
	if !strings.Contains(out.Rendered, "one month") {
		t.Errorf("6.3 years must yield one month notice, got: %s", out.Summary)
	}
}

func TestPayeMonthlyDefault(t *testing.T) {
	p := &Params{Computation: "paye", GrossIncome: f(450_000)}
	out, ok := Computation(p, "I earn 450,000 monthly, how much tax should they deduct?", now)
	if !ok {
		t.Fatal("expected computation")
	}
	if !strings.Contains(out.Rendered, "₦63,500 per month") {
		t.Errorf("450k monthly must yield ₦63,500/month, got: %s", out.Rendered)
	}
}

func TestParseParamsToleratesFences(t *testing.T) {
	raw := "```json\n{\"computation\": \"paye\", \"gross_income\": 450000, \"income_period\": \"monthly\"}\n```\nNote: extracted."
	p, err := ParseParams(raw)
	if err != nil || p.Computation != "paye" {
		t.Fatalf("ParseParams failed: %v %+v", err, p)
	}
}

// Captured misroutes (golden run 2026-07-02T113658): a minimum-wage
// legality question with a salary routed to PAYE; a maternity question
// with tenure routed to notice. The intent gate must block both.
func TestIntentGateBlocksOffTopicComputation(t *testing.T) {
	p := &Params{Computation: "paye", GrossIncome: f(45_000)}
	if _, ok := Computation(p, "Is my employer breaking the minimum wage law?", now); ok {
		t.Error("paye must not run for a question that never mentions tax")
	}
	p2 := &Params{Computation: "termination_notice", EmploymentYears: f(2)}
	if _, ok := Computation(p2, "Wetin be my right for maternity?", now); ok {
		t.Error("notice must not run for a maternity question")
	}
}

func TestNoComputationFallsThrough(t *testing.T) {
	p := &Params{Computation: "none"}
	if _, ok := Computation(p, "how much notice and tax do I get if they sack me?", now); ok {
		t.Error("'none' must fall through to lookup")
	}
}

// Captured failure (eval CP05): the extractor missed an "18 months" tenure
// entirely. When it extracts nothing, the question's own explicit duration
// must be parsed deterministically.
func TestQuestionDurationFallback(t *testing.T) {
	cases := []struct {
		question string
		want     string // substring of the rendered notice band
	}{
		{"I have worked there for 18 months, how much notice before they sack me?", "one week"},
		{"I don work there for three years, how much notice dem go give me?", "two weeks"},
		{"After eight years of service, what notice am I entitled to before dismissal?", "one month"},
		{"I resumed just 2 months ago — what notice applies if they fire me?", "one day"},
	}
	for _, c := range cases {
		p := &Params{Computation: "termination_notice"} // extractor got nothing
		out, ok := Computation(p, c.question, now)
		if !ok {
			t.Errorf("%q: expected computation via question-duration fallback", c.question)
			continue
		}
		if !strings.Contains(out.Rendered, c.want) {
			t.Errorf("%q: want %q in rendered, got: %s", c.question, c.want, out.Summary)
		}
	}
}

func TestDurationFromText(t *testing.T) {
	cases := []struct {
		text   string
		months float64
		ok     bool
	}{
		{"18 months", 18, true},
		{"three years", 36, true},
		{"a year", 12, true},
		{"2.5 years", 30, true},
		{"worked there since forever", 0, false},
		{"eighteen-months", 18, true},
		{"two and a half years at my job", 30, true}, // eval H13
	}
	for _, c := range cases {
		got, ok := durationFromText(c.text)
		if ok != c.ok || (ok && got != c.months) {
			t.Errorf("durationFromText(%q) = %v,%v want %v,%v", c.text, got, ok, c.months, c.ok)
		}
	}
}

// Captured misroute (eval 2026-07-03 TN02): "how much notice must a
// landlord give a yearly tenant" passed the notice gate and computed
// LABOUR Act bands for a TENANCY question — a wrong-law answer invisible
// to recall metrics. Tenancy contexts must veto the notice computation.
func TestTenancyVetoBlocksNoticeComputation(t *testing.T) {
	p := &Params{Computation: "termination_notice", EmploymentYears: f(1)}
	if _, ok := Computation(p, "How much notice must a landlord give a yearly tenant in Lagos?", now); ok {
		t.Error("landlord/tenant notice must not route to the Labour Act computation")
	}
	if _, ok := Computation(p, "My landlord served me a notice to quit after 1 year. Is it valid?", now); ok {
		t.Error("notice-to-quit must not route to the Labour Act computation")
	}
}

// Captured invention (eval 2026-07-03 XD05): "I earn exactly the minimum
// wage — do I pay income tax?" routed to PAYE with a ₦70,000 figure the
// model made up. Computed figures must be traceable to the question.
func TestInventedFiguresDoNotCompute(t *testing.T) {
	p := &Params{Computation: "paye", GrossIncome: f(70_000)}
	if _, ok := Computation(p, "I earn exactly the minimum wage in Nigeria. Do I pay income tax on it?", now); ok {
		t.Error("paye must not compute from an income the question never stated")
	}
	p2 := &Params{Computation: "termination_notice", EmploymentYears: f(3)}
	if _, ok := Computation(p2, "They sacked me without notice, wetin I fit do?", now); ok {
		t.Error("notice must not compute from a tenure the question never stated")
	}
}

// Spelled-out figures ARE question evidence (eval H15: "seventy thousand
// naira" blocked by the digit guard despite the model extracting it right).
func TestSpelledOutFiguresCompute(t *testing.T) {
	p := &Params{Computation: "paye", GrossIncome: f(70_000)}
	out, ok := Computation(p, "My monthly pay is seventy thousand naira, the national minimum wage. Should PAYE tax be deducted?", now)
	if !ok {
		t.Fatal("spelled-out income must count as question evidence")
	}
	if !strings.Contains(out.Summary, "exempt") {
		t.Errorf("minimum-wage earner must be exempt, got: %s", out.Summary)
	}
}

// The fallback must not override values the model DID extract — model years
// still outrank a stray question duration.
func TestModelYearsOutrankQuestionDuration(t *testing.T) {
	p := &Params{Computation: "termination_notice", EmploymentYears: f(6)}
	out, ok := Computation(p, "I got 2 months salary owed; they want to sack me after my 6 years, what notice?", now)
	if !ok {
		t.Fatal("expected computation")
	}
	if !strings.Contains(out.Rendered, "one month") {
		t.Errorf("6 years must yield one month notice, got: %s", out.Summary)
	}
}
