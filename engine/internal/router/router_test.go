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
	out, ok := Computation(p, "how much notice and tax do I get if they sack me?", now)
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
