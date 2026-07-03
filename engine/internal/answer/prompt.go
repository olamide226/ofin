// Package answer builds the citation-enforcing prompt from retrieved chunks.
package answer

import (
	"fmt"
	"strings"

	"ofin/internal/retrieve"
	"ofin/internal/verify"
)

// SystemPrompt is the citation-enforcing system instruction. Week 5 diet
// trimmed it ~30% without losing any of the six behavioral rules.
const SystemPrompt = `You are Òfin, a Nigerian legal information assistant.
1. Answer ONLY from the provided statutory SOURCES. Never use outside knowledge.
2. Every legal claim must end with a citation in the exact format [Act, s.X] or [Act, s.X(Y)], matching SOURCE labels.
3. Answer what the sources DO establish. "What can I do?" means: explain rights and remedies the sources give the user in their situation. If ANY source bears on the question, answer from it — even partially.
4. ONLY when the question belongs to an area of law none of the sources touch (criminal charges, divorce, immigration, etc.), reply: "The provided statutes do not cover this area of law." Never stretch an unrelated section to fit such a question.
5. If a source is state law (jurisdiction noted in the label), state which state it applies to.
6. Users may write in English or Nigerian Pidgin. Reply in the language they used.
7. Be concise and practical. You provide legal information, not legal advice.`

// PidginDirective overrides rule 6 when the user turns on Pidgin-first
// answers: reply in Pidgin no matter what language the question used.
// Citations stay in the exact bracket format — the verifier depends on it.
const PidginDirective = `
IMPORTANT OVERRIDE of rule 6: reply ONLY in Nigerian Pidgin, whatever language the question used. Keep citations exactly in the [Act, s.X] format.`

const (
	fullChunkChars  = 3000 // per-source cap for fused-rank sources
	hopChunkChars   = 800  // per-source cap for cross-ref hop companions
)

// BuildUserMessage assembles SOURCES + QUESTION. The first topN fused-rank
// sources get full text; remaining sources (cross-ref hop companions) get
// summary + truncated text — they are supplementary context, and cutting
// them protects prefill latency on the 8 GB target.
func BuildUserMessage(question string, chunks []retrieve.Chunk, topN int) string {
	var b strings.Builder
	b.WriteString("SOURCES:\n\n")
	for i, c := range chunks {
		title := ""
		if c.SectionTitle.Valid && c.SectionTitle.String != "" {
			title = " — " + c.SectionTitle.String
		}
		jur := ""
		if c.Jurisdiction != "federal" {
			jur = fmt.Sprintf(" (jurisdiction: %s)", c.Jurisdiction)
		}
		isHop := i >= topN
		cap := fullChunkChars
		if isHop {
			cap = hopChunkChars
		}
		text := c.Text
		if len(text) > cap {
			text = text[:cap] + " …"
		}
		summary := ""
		if c.Summary != "" {
			summary = "Summary: " + c.Summary + "\n"
		}
		if isHop {
			fmt.Fprintf(&b, "SOURCE %s%s%s (as at %s, see also):\n%s%s\n\n",
				c.Citation(), title, jur, c.AsAt, summary, text)
		} else {
			fmt.Fprintf(&b, "SOURCE %s%s%s (as at %s):\n%s%s\n\n",
				c.Citation(), title, jur, c.AsAt, summary, text)
		}
	}
	fmt.Fprintf(&b, "QUESTION: %s", question)
	return b.String()
}

// BuildCorrectionMessage constructs the single-retry regeneration prompt.
func BuildCorrectionMessage(failed []verify.Result) string {
	var b strings.Builder
	b.WriteString("VERIFICATION FAILED for these claims in your answer:\n\n")
	for i, r := range failed {
		fmt.Fprintf(&b, "%d. CLAIM: %s\n", i+1, r.Claim.Text)
		for _, reason := range r.Reasons {
			fmt.Fprintf(&b, "   PROBLEM: %s\n", reason)
		}
		if r.SourceText != "" {
			text := r.SourceText
			// 1200: the regeneration prompt rides on top of the full
			// SOURCES block and must fit the context window (observed
			// 6383 tokens vs 6144 ctx on tax questions at 2500).
			if len(text) > 1200 {
				text = text[:1200] + " …"
			}
			fmt.Fprintf(&b, "   ACTUAL TEXT OF %s:\n   %s\n", r.SourceRef, text)
		}
		b.WriteString("\n")
	}
	b.WriteString("Rewrite your complete answer. Keep only claims the statutory text " +
		"above and the original SOURCES support, with the same citation format. " +
		"If the sources do not answer part of the question, say so instead of guessing.")
	return b.String()
}
