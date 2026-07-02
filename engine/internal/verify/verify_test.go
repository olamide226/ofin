package verify

import (
	"fmt"
	"testing"
)

// Real statutory text (Labour Act s.11 / ECA s.4) — the sections our
// captured Week-2 failures cited.
const s11 = `11. (1) Either party to a contract of employment may terminate the contract on the expiration of notice given by him to the other party of his intention to do so.
(2) The notice to be given for the purposes of subsection (1) of this section shall be- (a) one day, where the contract has continued for a period of three months or less; (b) one week, where the contract has continued for more than three months but less than two years; (c) two weeks, where the contract has continued for a period of two years but less than five years; and (d) one month, where the contract has continued for five years or more.
(6) Nothing in this section shall prevent either party to a contract from waiving his right to notice on any occasion, or from accepting a payment in lieu of notice.`

const ecaS4 = `4. (1) In every case of an injury or disabling occupational disease to an employee in a workplace within the scope of this Act, the employee, or in case of death the dependant, shall within 14 days of the occurrence or receipt of the information of the occurrence, inform the employer by giving information of the disease or injury to a manager, supervisor, first aid attendant, agent in charge of the work where the injury occurred or other appropriate representative of the employer.`

type fakeCorpus map[string]string

func (f fakeCorpus) SectionText(act, section string) (string, error) {
	if t, ok := f[act+"|"+section]; ok {
		return t, nil
	}
	return "", fmt.Errorf("no such section")
}

// fakeEmbedder returns a fixed high-similarity vector for every text, so
// unit tests exercise the existence + quantity layers deterministically.
type fakeEmbedder struct{}

func (fakeEmbedder) Embed(string) ([]float32, error) { return []float32{1, 0, 0}, nil }

func newTestVerifier() *Verifier {
	return &Verifier{
		Corpus: fakeCorpus{
			"Labour Act 2004|s.11":                 s11,
			"Employees' Compensation Act 2010|s.4": ecaS4,
		},
		Embed: fakeEmbedder{},
	}
}

func TestParseCitations(t *testing.T) {
	cases := []struct {
		in  string
		act string
		sec string
		sub string
	}{
		{"blah [Labour Act 2004, s.11(2)(c)] end", "Labour Act 2004", "s.11", "(2)(c)"},
		{"[NMW Act 2019, s.4]", "NMW Act 2019", "s.4", ""},
		{"[Labour Act 2004, section 11]", "Labour Act 2004", "s.11", ""},
		{"[Employees' Compensation Act 2010, sch.3]", "Employees' Compensation Act 2010", "sch.3", ""},
	}
	for _, c := range cases {
		got := ParseCitations(c.in)
		if len(got) != 1 || got[0].Act != c.act || got[0].Section != c.sec || got[0].SubPath != c.sub {
			t.Errorf("ParseCitations(%q) = %+v, want %s %s %s", c.in, got, c.act, c.sec, c.sub)
		}
	}
}

// Captured failure: invented citation "s.7(8)" must fail existence.
func TestInventedCitationFails(t *testing.T) {
	v := newTestVerifier()
	res := v.VerifyClaim("", Claim{
		Text:      "you are entitled to receive your wages [Labour Act 2004, s.7(8)]",
		Citations: ParseCitations("[Labour Act 2004, s.7(8)]"),
	})
	if res.Verdict != Failed {
		t.Fatalf("invented citation: verdict %v, want Failed", res.Verdict)
	}
}

// Captured failure: "4 years -> one month" cites s.11 but contradicts the
// two-week band. "one month" DOES appear in s.11 (the 5-year band), so the
// quantity layer alone cannot catch band mis-mapping — that is the rules
// engine's job (Week 4). What quantity checking MUST catch is a value that
// appears nowhere in the section:
func TestQuantityAbsentFromSourceFails(t *testing.T) {
	v := newTestVerifier()
	// Captured failure: "notify within 7 days" against ECA s.4's 14 days.
	res := v.VerifyClaim("I got injured at work yesterday, what must I do first?", Claim{
		Text:      "You must notify your employer in writing within 7 days of the injury [Employees' Compensation Act 2010, s.4]",
		Citations: ParseCitations("[Employees' Compensation Act 2010, s.4]"),
	})
	if res.Verdict != Failed {
		t.Fatalf("7-days claim: verdict %v (reasons %v), want Failed", res.Verdict, res.Reasons)
	}
}

func TestSupportedQuantityPasses(t *testing.T) {
	v := newTestVerifier()
	res := v.VerifyClaim("I have worked for 3 years, how much notice am I owed?", Claim{
		Text:      "With 3 years of service you are entitled to two weeks of notice [Labour Act 2004, s.11(2)(c)]",
		Citations: ParseCitations("[Labour Act 2004, s.11(2)(c)]"),
	})
	if res.Verdict != Verified {
		t.Fatalf("two-weeks claim: verdict %v (reasons %v), want Verified", res.Verdict, res.Reasons)
	}
}

func TestSegmentClaims(t *testing.T) {
	answer := "Intro sentence. The notice is two weeks [Labour Act 2004, s.11(2)(c)]. " +
		"Payment in lieu is allowed [Labour Act 2004, s.11(6)]. You should see a lawyer."
	claims, uncited := SegmentClaims(answer, nil)
	if len(claims) != 2 {
		t.Fatalf("claims = %d, want 2", len(claims))
	}
	if len(uncited) != 1 {
		t.Fatalf("uncited = %v, want 1 trailing sentence", uncited)
	}
	// The intro sentence attaches forward to the first claim.
	if want := "Intro sentence"; claims[0].Text[:len(want)] != want {
		t.Errorf("first claim should absorb intro: %q", claims[0].Text)
	}
}

// testResolver mimics Store.ResolveAct for the acts used in fixtures.
func testResolver(raw string) (string, bool) {
	n := " " + raw + " "
	switch {
	case containsFold(n, "labour act"):
		return "Labour Act 2004", true
	case containsFold(n, "minimum wage"):
		return "NMW Act 2019", true
	case containsFold(n, "compensation act"):
		return "Employees' Compensation Act 2010", true
	}
	return "", false
}

func containsFold(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if eqFold(s[i:i+len(sub)], sub) {
				return true
			}
		}
		return false
	})()
}

func eqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 32
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// Real zero-receipt answers from the 2026-07-02 golden run: prose-cited
// answers the bracket-only parser missed entirely.
func TestProseCitationsParsed(t *testing.T) {
	cases := []struct {
		answer string
		act    string
		sec    string
	}{
		{"According to Section 11(3) of the Labour Act 2004, for a contract that has continued for two years, the notice period is two weeks.",
			"Labour Act 2004", "s.11"},
		{"According to Section 3 of the National Minimum Wage Act, 2019, the current national minimum wage in Nigeria is N70,000.00 per month.",
			"NMW Act 2019", "s.3"},
		{"According to Section 6 of the Employees' Compensation Act 2010, an application for compensation must be made to the employer.",
			"Employees' Compensation Act 2010", "s.6"},
	}
	for _, c := range cases {
		claims, _ := SegmentClaims(c.answer, testResolver)
		if len(claims) != 1 {
			t.Fatalf("prose answer produced %d claims: %q", len(claims), c.answer)
		}
		found := false
		for _, cit := range claims[0].Citations {
			if cit.Act == c.act && cit.Section == c.sec {
				found = true
			}
		}
		if !found {
			t.Errorf("wanted %s %s in %+v", c.act, c.sec, claims[0].Citations)
		}
	}
}

// L13-style: act named once, then bare refs "(s. 54(1)(a))" in later items.
func TestBareRefsBindToContextAct(t *testing.T) {
	answer := "According to Section 54 of the Labour Act 2004, a pregnant employee is entitled to leave. " +
		"She may leave work six weeks before confinement if medically certified (s. 54(1)(a)). " +
		"She must receive at least 50% of wages (s. 54(1)(c))."
	claims, uncited := SegmentClaims(answer, testResolver)
	if len(uncited) != 0 {
		t.Errorf("all sentences are cited, uncited = %v", uncited)
	}
	for _, cl := range claims {
		ok := false
		for _, cit := range cl.Citations {
			if cit.Act == "Labour Act 2004" && cit.Section == "s.54" {
				ok = true
			}
		}
		if !ok {
			t.Errorf("claim %q missing bound s.54 citation: %+v", cl.Text, cl.Citations)
		}
	}
}

// Regression from golden run 2026-07-02T050330: numbered-list answers must
// not shed "1"/"2" fragments as claims, and a trailing citation-only line
// belongs to the preceding text.
func TestListAnswersSegmentCleanly(t *testing.T) {
	answer := "A pregnant employee is entitled to: 1. Leave six weeks before confinement (s. 54(1)(a)). " +
		"2. Half an hour twice a day for nursing (s. 54(1)(d)). This comes from the Labour Act 2004.\n" +
		"[Labour Act 2004, s.54(1)]"
	claims, uncited := SegmentClaims(answer, testResolver)
	for _, c := range claims {
		if listMarkerRe.MatchString(c.Text) {
			t.Errorf("bare list marker became a claim: %q", c.Text)
		}
		if StripCitations(c.Text) == "" {
			t.Errorf("citation-only fragment became a claim: %q", c.Text)
		}
	}
	_ = uncited
}

func TestExtractQuantities(t *testing.T) {
	qs := ExtractQuantities("pay of N70,000.00 per month and 12 working days within 14 days at 5% interest")
	want := map[string]bool{"70000naira": true, "12day": true, "14day": true, "5percent": true}
	for _, q := range qs {
		key := fmt.Sprintf("%g%s", q.Value, q.Unit)
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("missing quantities: %v (got %v)", want, qs)
	}
}

func TestStripCitationsRemovesSectionRefs(t *testing.T) {
	in := "Under section 11 of the Labour Act the notice is two weeks [Labour Act 2004, s.11(2)(c)]"
	out := StripCitations(in)
	if ExtractQuantities(out)[0].Unit != "week" {
		t.Errorf("section numbers must not survive as quantities: %q -> %v", out, ExtractQuantities(out))
	}
}
