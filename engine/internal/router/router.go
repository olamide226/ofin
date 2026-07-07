// Package router classifies questions as lookup vs computation and, for
// computations, drives the neuro-symbolic split: the LLM extracts
// parameters as JSON, the rules engine computes, the LLM narrates the
// computed result. The model never does arithmetic (Pillar 2).
package router

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ofin/internal/rules"
)

// ExtractionPrompt mirrors the bake-off X-format both finalist models
// handled well: bare JSON, fixed schema, no prose.
const ExtractionPrompt = `You are the parameter extractor for a legal computation engine. Read the user's message and output ONLY a JSON object matching this schema, with no other text:
{"computation": "paye"|"termination_notice"|"tenancy_notice"|"redundancy"|"none",
 "gross_income": number|null, "income_period": "monthly"|"annual"|null,
 "employment_years": number|null, "employment_start": string|null,
 "annual_rent": number|null,
 "tenancy_type": "at_will"|"monthly"|"quarterly"|"half_yearly"|"yearly"|"fixed_term"|null}
Rules: "computation" is "paye" for tax questions with an income figure, "termination_notice" for how-much-notice questions about a JOB with a tenure, "tenancy_notice" for landlord/tenant notice-to-quit questions, "redundancy" for redundancy/retrenchment/layoff entitlement questions, otherwise "none". Numbers must be plain digits. Durations stated in months convert to years ("18 months" -> "employment_years": 1.5). Do not guess values not present in the message.`

// Params is what the LLM extracts.
type Params struct {
	Computation     string   `json:"computation"`
	GrossIncome     *float64 `json:"gross_income"`
	IncomePeriod    *string  `json:"income_period"`
	EmploymentYears *float64 `json:"employment_years"`
	EmploymentStart *string  `json:"employment_start"`
	AnnualRent      *float64 `json:"annual_rent"`
	TenancyType     *string  `json:"tenancy_type"`
}

var jsonBlockRe = regexp.MustCompile(`\{[\s\S]*\}`)

// ParseParams tolerates markdown fences and stray prose around the JSON
// (Phi-style wrapping was a known bake-off failure mode).
func ParseParams(raw string) (*Params, error) {
	m := jsonBlockRe.FindString(raw)
	if m == "" {
		return nil, fmt.Errorf("no JSON object in extraction output")
	}
	var p Params
	if err := json.Unmarshal([]byte(m), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Outcome is a completed computation: machine-readable JSON, the
// deterministically rendered user-facing answer, and a one-line summary.
type Outcome struct {
	JSON     string
	Rendered string
	Summary  string
	Kind     string
}

// Intent gates: the extractor routes on the PRESENCE of numbers, not on
// what the question asks (observed: a Pidgin maternity question with "two
// years" tenure routed to notice; a minimum-wage legality question with a
// salary routed to PAYE). A computation runs only when the question's own
// words ask for it.
var intentGates = map[string]*regexp.Regexp{
	"paye":               regexp.MustCompile(`(?i)\b(tax|paye)\b`),
	"termination_notice": regexp.MustCompile(`(?i)notice|sack|dismiss|terminat|fire|lay.?off|resign`),
	"tenancy_notice":     regexp.MustCompile(`(?i)notice|quit|evict|eject|pack out|leave|end.{0,12}tenanc`),
	"redundancy":         regexp.MustCompile(`(?i)redundan|retrench|downsiz|excess of manpower|lay.?off|laid off`),
}

// tenancyVeto blocks the LABOUR notice computation for landlord/tenant
// questions: "how much notice must a landlord give" passes the notice gate
// but belongs to the Lagos Tenancy Law, not Labour Act s.11 (eval TN02
// misroute — a wrong-law computed answer recall metrics cannot see).
var tenancyVeto = regexp.MustCompile(`(?i)\b(landlord|tenant|tenancy|premises|quit)\b`)

var digitRe = regexp.MustCompile(`\d`)

// Computation runs the rules engine for the extracted parameters.
// ok=false means the question falls through to the normal lookup path.
func Computation(p *Params, question string, now time.Time) (Outcome, bool) {
	kind := p.Computation
	// Kind normalization: the extractor sees "notice" and says
	// termination_notice even for landlord/tenant questions (eval TN02) and
	// redundancy questions (notice is only half that answer). The question's
	// own domain words win.
	if kind == "termination_notice" {
		if tenancyVeto.MatchString(question) {
			kind = "tenancy_notice"
		} else if intentGates["redundancy"].MatchString(question) {
			kind = "redundancy"
		}
	}
	if gate, known := intentGates[kind]; !known || !gate.MatchString(question) {
		return Outcome{}, false
	}
	switch kind {
	case "termination_notice":
		months, ok := tenureMonths(p, question, now)
		if !ok {
			return Outcome{}, false
		}
		res := rules.NoticePeriod(months)
		out, _ := json.MarshalIndent(res, "", " ")
		return Outcome{JSON: string(out), Rendered: res.Render(),
			Summary: res.Summary(), Kind: "termination_notice"}, true

	case "tenancy_notice":
		// Tenancy type must be traceable to the question text (ADR-010
		// applies to inputs): deterministic parse, no model value accepted.
		t, ok := tenancyTypeFromText(question)
		if !ok {
			return Outcome{}, false
		}
		res, ok := rules.TenancyNotice(t)
		if !ok {
			return Outcome{}, false
		}
		out, _ := json.MarshalIndent(res, "", " ")
		return Outcome{JSON: string(out), Rendered: res.Render(),
			Summary: res.Summary(), Kind: "tenancy_notice"}, true

	case "redundancy":
		months, _ := tenureMonths(p, question, now) // tenure optional: rights render without it
		res := rules.Redundancy(months)
		out, _ := json.MarshalIndent(res, "", " ")
		return Outcome{JSON: string(out), Rendered: res.Render(),
			Summary: res.Summary(), Kind: "redundancy"}, true

	case "paye":
		if p.GrossIncome == nil || *p.GrossIncome <= 0 {
			// No income figure — user asked "how is tax calculated?"
			// conceptually. Render band structure without a specific income.
			return Outcome{Kind: "paye_conceptual",
				Rendered: rules.PAYEConceptual(),
				Summary:   "PAYE tax bands and formula"}, true
		}
		// The income figure must be traceable to the question: the
		// extractor was observed inventing ₦70,000 for "I earn exactly the
		// minimum wage" (eval XD05) — a computed answer built on a number
		// the user never gave. ADR-010 applies to inputs, not just math.
		// Digits or spelled-out amounts both count as evidence.
		if !digitRe.MatchString(question) && !moneyEvidenceRe.MatchString(question) {
			return Outcome{}, false
		}
		annual := *p.GrossIncome
		if p.IncomePeriod == nil || *p.IncomePeriod != "annual" {
			annual *= 12 // default: figures are monthly unless stated annual
		}
		in := rules.PAYEInput{GrossAnnual: annual}
		if p.AnnualRent != nil {
			in.AnnualRent = *p.AnnualRent
		}
		res := rules.PAYE(in)
		out, _ := json.MarshalIndent(res, "", " ")
		summary := fmt.Sprintf("annual tax ₦%.0f, monthly ₦%.0f", res.AnnualTax, res.MonthlyTax)
		if res.Exempt {
			summary = "exempt from PAYE"
		}
		return Outcome{JSON: string(out), Rendered: res.Render(),
			Summary: summary, Kind: "paye"}, true
	}
	return Outcome{}, false
}

var monthNames = map[string]time.Month{
	"january": 1, "february": 2, "march": 3, "april": 4, "may": 5, "june": 6,
	"july": 7, "august": 8, "september": 9, "october": 10, "november": 11, "december": 12,
}

var yearRe = regexp.MustCompile(`\b(19|20)\d{2}\b`)

func tenureMonths(p *Params, question string, now time.Time) (float64, bool) {
	// A start date outranks a model-supplied duration: the extractor was
	// observed inventing employment_years=3 for "since March 2020" (6.3
	// years). Date arithmetic is ours, never the model's.
	if p.EmploymentStart != nil {
		if months, ok := tenureFromStart(*p.EmploymentStart, now); ok {
			return months, true
		}
	}
	// Model-extracted years count only when the question itself contains
	// SOME duration evidence (digits or a spelled-out duration) — the
	// extractor invents figures for questions that state none (ADR-010
	// applies to inputs, not just math).
	qMonths, qOK := durationFromText(question)
	if p.EmploymentYears != nil && *p.EmploymentYears > 0 &&
		(qOK || digitRe.MatchString(question)) {
		return *p.EmploymentYears * 12, true
	}
	// Fallback: the extractor was observed missing month-denominated
	// tenures entirely ("18 months", eval CP05). When it extracted nothing,
	// use the question's first explicit duration, parsed deterministically.
	return qMonths, qOK
}

var wordNumbers = map[string]float64{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
	"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11,
	"twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15,
	"sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19,
	"twenty": 20, "a": 1, "an": 1,
}

var durationRe = regexp.MustCompile(
	`(?i)\b(\d+(?:\.\d+)?|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty|an?)(\s+and\s+a\s+half)?[\s-]+(years?|months?)\b`)

// durationFromText parses the first explicit duration in a question into
// months. Digits or number words, years or months, "and a half" ("18
// months", "three years", "two and a half years", "a year"). First match
// only — summing multiple mentions would conflate tenure with an unrelated
// duration in the same sentence.
func durationFromText(text string) (float64, bool) {
	m := durationRe.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	var n float64
	if v, ok := wordNumbers[strings.ToLower(m[1])]; ok {
		n = v
	} else if _, err := fmt.Sscanf(m[1], "%g", &n); err != nil {
		return 0, false
	}
	if n <= 0 {
		return 0, false
	}
	if m[2] != "" {
		n += 0.5
	}
	if strings.HasPrefix(strings.ToLower(m[3]), "year") {
		n *= 12
	}
	return n, true
}

// moneyEvidenceRe: a question stating an amount in words ("seventy thousand
// naira") is evidence for a model-extracted income even without digits
// (eval H15 — the guard must not block spelled-out figures).
var moneyEvidenceRe = regexp.MustCompile(`(?i)\b(thousand|million|billion)\b`)

// tenancyTypePatterns, in match order: half-yearly MUST precede yearly
// ("half-yearly" contains a word-boundary "yearly"), and specific forms
// precede generic ones.
var tenancyTypePatterns = []struct {
	t  rules.TenancyType
	re *regexp.Regexp
}{
	{rules.TenancyFixedTerm, regexp.MustCompile(`(?i)fixed[\s-]?term|term (has )?(expired|ended)|efflux`)},
	{rules.TenancyAtWill, regexp.MustCompile(`(?i)\bat[\s-]?will\b`)},
	{rules.TenancyHalfYearly, regexp.MustCompile(`(?i)half[\s-]?year|every six months|six[\s-]?monthly|bi[\s-]?annual`)},
	{rules.TenancyQuarterly, regexp.MustCompile(`(?i)\bquarter`)},
	{rules.TenancyYearly, regexp.MustCompile(`(?i)\byearly\b|\bannual|\bper year\b|\bevery year\b`)},
	{rules.TenancyMonthly, regexp.MustCompile(`(?i)\bmonthly\b|\bper month\b|\bevery month\b|\bmonth to month\b`)},
}

// tenancyTypeFromText classifies the tenancy deterministically from the
// question's own words (s.13(6): rent frequency determines tenancy nature).
func tenancyTypeFromText(question string) (rules.TenancyType, bool) {
	for _, p := range tenancyTypePatterns {
		if p.re.MatchString(question) {
			return p.t, true
		}
	}
	return "", false
}

func tenureFromStart(startRaw string, now time.Time) (float64, bool) {
	raw := strings.ToLower(startRaw)
	ym := yearRe.FindString(raw)
	if ym == "" {
		return 0, false
	}
	var year int
	fmt.Sscanf(ym, "%d", &year)
	month := time.January // conservative default: assume January
	for name, m := range monthNames {
		if strings.Contains(raw, name) {
			month = m
			break
		}
	}
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	if start.After(now) {
		return 0, false
	}
	return now.Sub(start).Hours() / 24 / 30.44, true
}

// NarrationSystemPrompt instructs the model to present, not compute.
const NarrationSystemPrompt = `You are Òfin, a Nigerian legal information assistant. The deterministic computation engine has ALREADY computed the result below directly from statute. Your job is to present it clearly:
1. State the outcome first, then the breakdown (as a list), using EXACTLY the numbers in COMPUTATION RESULT — never recompute, round differently, or invent figures.
2. Keep every citation shown in the result, in the format [Act, s.X].
3. If SOURCES are provided, you may add brief relevant context from them, cited.
4. Users may write in English or Nigerian Pidgin; reply in the user's language.
5. Be concise. You provide legal information, not legal advice.`

// BuildNarrationMessage assembles the user message for the narration call.
func BuildNarrationMessage(question, resultJSON, sources string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "COMPUTATION RESULT:\n%s\n\n", resultJSON)
	if sources != "" {
		fmt.Fprintf(&b, "%s\n\n", sources)
	}
	fmt.Fprintf(&b, "QUESTION: %s", question)
	return b.String()
}
