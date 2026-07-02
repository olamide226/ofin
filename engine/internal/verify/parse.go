// Package verify implements the Verified Citation Engine: parse citation
// tokens out of model output, check every cited claim against the local
// corpus, and return per-claim verdicts. Deterministic wherever possible —
// the whole point is that this layer does not hallucinate.
package verify

import (
	"regexp"
	"strings"
)

// Citation is one [Act, s.X(Y)] token as written by the model.
type Citation struct {
	Act     string // e.g. "Labour Act 2004"
	Section string // base section id used for lookup, e.g. "s.11" or "sch.1"
	SubPath string // subsection path for display, e.g. "(2)(c)"
	Raw     string
}

// Claim is a contiguous span of answer text carrying one or more citations.
type Claim struct {
	Text      string
	Citations []Citation
}

var citationRe = regexp.MustCompile(
	`\[([^,\[\]]+?),\s*(?:[Ss]ection\s+|[Ss]s?\.\s*|sch\.\s*)(\d+[A-Za-z]?)((?:\(\w{1,4}\))*)\]`)

var schRe = regexp.MustCompile(`\[([^,\[\]]+?),\s*sch\.\s*(\d+)`)

// Prose citation: "Section 11(3) of the Labour Act 2004", "section 4 of the
// National Minimum Wage Act, 2019". 3B models drift into this form no
// matter what the prompt demands, so the verifier meets them there.
var proseRe = regexp.MustCompile(
	`(?i)\bsections?\s+(\d+[A-Za-z]?)((?:\s*\(\w{1,4}\))*)\s+of\s+the\s+` +
		`([A-Z][\w'()]*(?:[\s,]+[\w'()]+){0,6}?\s(?:Act|Law)(?:,?\s*\d{4})?)`)

// Bare section ref "(s. 54(1)(a))" or "s.54(1)" — attaches to the most
// recently mentioned act in the answer.
var bareRefRe = regexp.MustCompile(
	`(?i)\(?\bs\.?\s*(\d+[A-Za-z]?)((?:\s*\(\w{1,4}\))*)\)?`)

// Act mention anywhere in prose, for bare-ref context: "the Labour Act
// 2004", "Employees' Compensation Act, 2010".
var actMentionRe = regexp.MustCompile(
	`[A-Z][\w'()]*(?:[\s,]+[\w'()]+){0,6}?\s(?:Act|Law),?\s*(?:\d{4})?`)

// ActResolver maps a prose act name to the canonical act_short used in the
// corpus ("National Minimum Wage Act, 2019" -> "NMW Act 2019"). ok=false
// when the name matches no known act — the citation then fails existence,
// which is the honest outcome.
type ActResolver func(raw string) (canonical string, ok bool)

// IdentityResolver passes act names through unchanged (tests, bracket-only).
func IdentityResolver(raw string) (string, bool) { return strings.TrimSpace(raw), true }

// ParseCitations extracts bracket-token citations only (the canonical form).
func ParseCitations(text string) []Citation {
	var out []Citation
	for _, m := range citationRe.FindAllStringSubmatch(text, -1) {
		prefix := "s."
		if schRe.MatchString(m[0]) {
			prefix = "sch."
		}
		out = append(out, Citation{
			Act:     strings.TrimSpace(m[1]),
			Section: prefix + strings.ToLower(m[2]),
			SubPath: strings.ReplaceAll(m[3], " ", ""),
			Raw:     m[0],
		})
	}
	return out
}

// ParseAllCitations extracts bracket AND prose citations, resolving prose
// act names through the resolver. Bare "(s.N)" refs bind to the nearest
// preceding act mention. Duplicates (same act+section) collapse.
func ParseAllCitations(text string, resolve ActResolver) []Citation {
	if resolve == nil {
		resolve = IdentityResolver
	}
	out := ParseCitations(text)
	seen := map[string]bool{}
	for _, c := range out {
		seen[c.Act+"|"+c.Section] = true
	}
	add := func(c Citation) {
		key := c.Act + "|" + c.Section
		if !seen[key] {
			seen[key] = true
			out = append(out, c)
		}
	}

	stripped := citationRe.ReplaceAllString(text, "")
	for _, m := range proseRe.FindAllStringSubmatch(stripped, -1) {
		if act, ok := resolve(m[3]); ok {
			add(Citation{Act: act, Section: "s." + strings.ToLower(m[1]),
				SubPath: strings.ReplaceAll(m[2], " ", ""), Raw: strings.TrimSpace(m[0])})
		}
	}

	// Bare refs: walk act mentions and section refs in order; each bare ref
	// binds to the last act mentioned before it.
	mentions := actMentionRe.FindAllStringIndex(stripped, -1)
	resolvedMentions := make([]struct {
		pos int
		act string
	}, 0, len(mentions))
	for _, span := range mentions {
		if act, ok := resolve(stripped[span[0]:span[1]]); ok {
			resolvedMentions = append(resolvedMentions, struct {
				pos int
				act string
			}{span[0], act})
		}
	}
	if len(resolvedMentions) > 0 {
		for _, span := range bareRefRe.FindAllStringSubmatchIndex(stripped, -1) {
			m := bareRefRe.FindStringSubmatch(stripped[span[0]:span[1]])
			ctx := ""
			for _, am := range resolvedMentions {
				if am.pos < span[0] {
					ctx = am.act
				}
			}
			if ctx == "" {
				continue
			}
			add(Citation{Act: ctx, Section: "s." + strings.ToLower(m[1]),
				SubPath: strings.ReplaceAll(m[2], " ", ""), Raw: stripped[span[0]:span[1]]})
		}
	}
	return out
}

var sentenceEndRe = regexp.MustCompile(`[.!?]\s+|\n+`)

// listMarkerRe matches fragments that are numbered/lettered list markers
// left behind by sentence splitting ("1", "2", "(a)", "iv").
var listMarkerRe = regexp.MustCompile(`^\(?[0-9ivxl]{1,3}\)?$|^\(?[a-h]\)$`)

// SegmentClaims splits an answer into claims (sentence groups ending in at
// least one citation — bracket or prose) and uncited spans. A sentence with
// no citation attaches forward: it usually introduces the cited sentence
// that follows; trailing uncited sentences are returned as unverified text.
// Act context for bare "(s.N)" refs carries across the whole answer.
func SegmentClaims(answer string, resolve ActResolver) (claims []Claim, uncited []string) {
	parts := sentenceEndRe.Split(answer, -1)
	var buf []string
	var lastAct string
	withContext := func(text string) string {
		// Prepend the running act mention so bare refs in this sentence
		// group can bind even when the act was named sentences earlier.
		if lastAct != "" && !actMentionRe.MatchString(text) {
			return lastAct + ": " + text
		}
		return text
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || listMarkerRe.MatchString(p) {
			continue // list markers are layout, not sentences
		}
		if m := actMentionRe.FindString(p); m != "" {
			if resolve != nil {
				if _, ok := resolve(m); ok {
					lastAct = m
				}
			} else {
				lastAct = m
			}
		}
		// A citation-only fragment ("[Labour Act 2004, s.11(3)]" on its own
		// line) is the citation FOR the preceding claim or buffer, not a
		// claim in itself.
		if StripCitations(p) == "" {
			if cits := ParseAllCitations(p, resolve); len(cits) > 0 {
				if len(buf) > 0 {
					text := strings.Join(append(buf, p), ". ")
					claims = append(claims, Claim{Text: text,
						Citations: ParseAllCitations(withContext(text), resolve)})
					buf = nil
				} else if len(claims) > 0 {
					last := &claims[len(claims)-1]
					last.Citations = append(last.Citations, cits...)
				}
			}
			continue
		}
		buf = append(buf, p)
		text := strings.Join(buf, ". ")
		if cits := ParseAllCitations(withContext(text), resolve); len(cits) > 0 {
			claims = append(claims, Claim{Text: text, Citations: cits})
			buf = nil
		}
	}
	for _, p := range buf {
		uncited = append(uncited, p)
	}
	return claims, uncited
}

// StripCitations removes citation tokens and bare section references from
// claim text so downstream checks (quantities, embeddings) see only the
// legal assertion itself.
var bareSectionRe = regexp.MustCompile(
	`(?i)\b(?:sections?|s\.|ss\.)\s*\d+[A-Za-z]?(?:\s*\(\w{1,4}\))*(?:\s+(?:of|to)\s+(?:this|the)\s+[\w\s]{0,30}Act)?`)

func StripCitations(text string) string {
	text = citationRe.ReplaceAllString(text, "")
	text = bareSectionRe.ReplaceAllString(text, "")
	return strings.Join(strings.Fields(text), " ")
}
